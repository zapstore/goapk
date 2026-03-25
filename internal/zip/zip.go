// Package zip assembles Android APK files as properly ordered ZIP archives.
//
// APK entry ordering:
//  1. AndroidManifest.xml  (DEFLATE)
//  2. classes.dex          (DEFLATE)
//  3. resources.arsc       (STORED — Android memory-maps this file)
//  4. res/*                (DEFLATE for PNG, STORED for .so)
//  5. assets/*             (DEFLATE)
//  6. lib/*                (STORED — loaded directly from ZIP)
//
// Compression rules:
//   - resources.arsc and .so files must be STORED for direct memory-mapping / dlopen.
//   - Everything else uses DEFLATE.
package zip

import (
	"archive/zip"
	"bytes"
	"compress/flate"
	"encoding/binary"
	"hash/crc32"
	"io"
	"strings"
	"time"
)

// Entry holds the data for a single APK ZIP entry.
type Entry struct {
	Name    string // path inside the APK (e.g. "res/mipmap-mdpi-v4/ic_launcher.png")
	Data    []byte // raw (uncompressed) content
	Stored  bool   // if true, use STORE; otherwise DEFLATE
}

// NewEntry creates an Entry with automatic compression selection:
// resources.arsc and .so files are STORED; everything else is DEFLATE.
func NewEntry(name string, data []byte) Entry {
	stored := shouldStore(name)
	return Entry{Name: name, Data: data, Stored: stored}
}

// shouldStore returns true for entries that must not be compressed.
func shouldStore(name string) bool {
	lower := strings.ToLower(name)
	return lower == "resources.arsc" ||
		strings.HasSuffix(lower, ".so") ||
		strings.HasSuffix(lower, ".arsc")
}

// Build assembles entries into a valid ZIP (APK) byte slice.
// It writes explicit sizes and CRCs in the local file headers (no data descriptors),
// which is required for zipalign to work correctly on STORED entries.
// A fixed modification time (2009-01-01) is used for reproducible output.
func Build(entries []Entry) ([]byte, error) {
	modTime := time.Date(2009, 1, 1, 0, 0, 0, 0, time.UTC)
	dosDate, dosTime := toDosTime(modTime)

	type localRecord struct {
		localHeaderOff int
		name           string
		method         uint16
		crc            uint32
		compSize       uint32
		uncompSize     uint32
	}

	var buf bytes.Buffer
	var records []localRecord

	for _, e := range entries {
		var compressed []byte
		method := uint16(8) // DEFLATE

		if e.Stored {
			compressed = e.Data
			method = 0
		} else {
			var cb bytes.Buffer
			fw, err := flate.NewWriter(&cb, flate.BestCompression)
			if err != nil {
				return nil, err
			}
			if _, err := fw.Write(e.Data); err != nil {
				return nil, err
			}
			if err := fw.Close(); err != nil {
				return nil, err
			}
			compressed = cb.Bytes()
		}

		crc := crc32.ChecksumIEEE(e.Data)
		nameBytes := []byte(e.Name)
		lhdrOff := buf.Len()

		// Local file header — sizes and CRC written explicitly (no data descriptor flag)
		lhdr := make([]byte, 30)
		binary.LittleEndian.PutUint32(lhdr[0:], 0x04034b50) // signature
		binary.LittleEndian.PutUint16(lhdr[4:], 20)         // version needed
		binary.LittleEndian.PutUint16(lhdr[6:], 0)          // flags (bit 3 NOT set)
		binary.LittleEndian.PutUint16(lhdr[8:], method)
		binary.LittleEndian.PutUint16(lhdr[10:], dosTime)
		binary.LittleEndian.PutUint16(lhdr[12:], dosDate)
		binary.LittleEndian.PutUint32(lhdr[14:], crc)
		binary.LittleEndian.PutUint32(lhdr[18:], uint32(len(compressed)))
		binary.LittleEndian.PutUint32(lhdr[22:], uint32(len(e.Data)))
		binary.LittleEndian.PutUint16(lhdr[26:], uint16(len(nameBytes)))
		binary.LittleEndian.PutUint16(lhdr[28:], 0) // extra len

		buf.Write(lhdr)
		buf.Write(nameBytes)
		buf.Write(compressed)

		records = append(records, localRecord{
			localHeaderOff: lhdrOff,
			name:           e.Name,
			method:         method,
			crc:            crc,
			compSize:       uint32(len(compressed)),
			uncompSize:     uint32(len(e.Data)),
		})
	}

	// Central directory
	cdStart := buf.Len()
	for _, r := range records {
		nameBytes := []byte(r.name)
		cd := make([]byte, 46)
		binary.LittleEndian.PutUint32(cd[0:], 0x02014b50) // CD signature
		binary.LittleEndian.PutUint16(cd[4:], 20)         // version made by
		binary.LittleEndian.PutUint16(cd[6:], 20)         // version needed
		binary.LittleEndian.PutUint16(cd[8:], 0)          // flags
		binary.LittleEndian.PutUint16(cd[10:], r.method)
		binary.LittleEndian.PutUint16(cd[12:], dosTime)
		binary.LittleEndian.PutUint16(cd[14:], dosDate)
		binary.LittleEndian.PutUint32(cd[16:], r.crc)
		binary.LittleEndian.PutUint32(cd[20:], r.compSize)
		binary.LittleEndian.PutUint32(cd[24:], r.uncompSize)
		binary.LittleEndian.PutUint16(cd[28:], uint16(len(nameBytes)))
		// extra(2)=0, comment(2)=0, diskStart(2)=0, intAttr(2)=0, extAttr(4)=0
		binary.LittleEndian.PutUint32(cd[42:], uint32(r.localHeaderOff))
		buf.Write(cd)
		buf.Write(nameBytes)
	}

	// End of Central Directory
	cdSize := buf.Len() - cdStart
	eocd := make([]byte, 22)
	binary.LittleEndian.PutUint32(eocd[0:], 0x06054b50) // EOCD signature
	binary.LittleEndian.PutUint16(eocd[8:], uint16(len(records)))
	binary.LittleEndian.PutUint16(eocd[10:], uint16(len(records)))
	binary.LittleEndian.PutUint32(eocd[12:], uint32(cdSize))
	binary.LittleEndian.PutUint32(eocd[16:], uint32(cdStart))
	buf.Write(eocd)

	return buf.Bytes(), nil
}

// RawEntry is a manually constructed ZIP local file entry, used when we need
// precise control over the local file header (e.g. for zipalign padding).
type RawEntry struct {
	Name             string
	CompressedData   []byte
	UncompressedData []byte
	Method           uint16 // 0=STORE, 8=DEFLATE
	CRC32            uint32
}

// BuildRaw constructs the APK bytes with full control over local file headers,
// enabling zipalign to pad the extra-data field.
// Returns the assembled bytes and the list of raw entries (for use by the signer).
func BuildRaw(entries []Entry) ([]byte, []RawEntry, error) {
	modTime := time.Date(2009, 1, 1, 0, 0, 0, 0, time.UTC)
	dosDate, dosTime := toDosTime(modTime)

	var buf bytes.Buffer
	var raws []RawEntry

	for _, e := range entries {
		var compressed []byte
		method := uint16(8) // DEFLATE

		if e.Stored {
			compressed = e.Data
			method = 0
		} else {
			var cb bytes.Buffer
			fw, err := flate.NewWriter(&cb, flate.BestCompression)
			if err != nil {
				return nil, nil, err
			}
			if _, err := fw.Write(e.Data); err != nil {
				return nil, nil, err
			}
			if err := fw.Close(); err != nil {
				return nil, nil, err
			}
			compressed = cb.Bytes()
		}

		crc := crc32.ChecksumIEEE(e.Data)
		nameBytes := []byte(e.Name)

		raw := RawEntry{
			Name:             e.Name,
			CompressedData:   compressed,
			UncompressedData: e.Data,
			Method:           method,
			CRC32:            crc,
		}
		raws = append(raws, raw)

		// Local file header (30 bytes + name + extra)
		lhdr := make([]byte, 30)
		binary.LittleEndian.PutUint32(lhdr[0:], 0x04034b50) // signature
		binary.LittleEndian.PutUint16(lhdr[4:], 20)         // version needed
		binary.LittleEndian.PutUint16(lhdr[6:], 0)          // flags
		binary.LittleEndian.PutUint16(lhdr[8:], method)
		binary.LittleEndian.PutUint16(lhdr[10:], dosTime)
		binary.LittleEndian.PutUint16(lhdr[12:], dosDate)
		binary.LittleEndian.PutUint32(lhdr[14:], crc)
		binary.LittleEndian.PutUint32(lhdr[18:], uint32(len(compressed)))
		binary.LittleEndian.PutUint32(lhdr[22:], uint32(len(e.Data)))
		binary.LittleEndian.PutUint16(lhdr[26:], uint16(len(nameBytes)))
		binary.LittleEndian.PutUint16(lhdr[28:], 0) // extra len = 0 (filled by zipalign)

		buf.Write(lhdr)
		buf.Write(nameBytes)
		buf.Write(compressed)
	}

	return buf.Bytes(), raws, nil
}

// ReadCentralDirectory parses the ZIP central directory from raw APK bytes.
// Returns the entries and the offset of the EOCD record.
func ReadCentralDirectory(data []byte) ([]CDEntry, int64, error) {
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, 0, err
	}

	var entries []CDEntry
	for _, f := range r.File {
		rc, err := f.Open()
		if err != nil {
			return nil, 0, err
		}
		content, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return nil, 0, err
		}
		entries = append(entries, CDEntry{
			Header:  f.FileHeader,
			Content: content,
		})
	}

	// Find EOCD offset
	eocdOff, err := findEOCDOffset(data)
	if err != nil {
		return nil, 0, err
	}
	return entries, eocdOff, nil
}

// CDEntry is a ZIP central directory entry with its content.
type CDEntry struct {
	Header  zip.FileHeader
	Content []byte
}

// findEOCDOffset searches backwards for the EOCD signature.
func findEOCDOffset(data []byte) (int64, error) {
	const sig = uint32(0x06054b50)
	// EOCD is at least 22 bytes; search backwards
	for i := len(data) - 22; i >= 0; i-- {
		if binary.LittleEndian.Uint32(data[i:]) == sig {
			return int64(i), nil
		}
	}
	return 0, io.ErrUnexpectedEOF
}

// toDosTime converts time.Time to MS-DOS date and time fields (ZIP format).
func toDosTime(t time.Time) (uint16, uint16) {
	date := uint16((t.Year()-1980)<<9 | int(t.Month())<<5 | t.Day())
	tim := uint16(t.Hour()<<11 | t.Minute()<<5 | t.Second()/2)
	return date, tim
}
