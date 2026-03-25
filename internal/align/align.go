// Package align implements zipalign: padding uncompressed (STORED) ZIP entries
// so their data starts at a multiple of the alignment boundary (default 4 bytes).
//
// Alignment is achieved by padding the "extra data" field in each local file header.
// The Central Directory is rebuilt with updated local header offsets to match.
//
// Reference: https://developer.android.com/tools/zipalign
package align

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
)

const (
	// DefaultAlignment is the standard zipalign boundary for Android APKs.
	DefaultAlignment = 4

	localFileSig  = uint32(0x04034b50)
	centralDirSig = uint32(0x02014b50)
	eocdSig       = uint32(0x06054b50)
)

// localEntry represents one parsed local file header.
type localEntry struct {
	// Fields from the original local file header
	versionNeeded  uint16
	flags          uint16
	method         uint16
	modTime        uint16
	modDate        uint16
	crc32          uint32
	compressedSize uint32
	uncompSize     uint32
	name           []byte
	extra          []byte
	data           []byte

	// Computed
	origOffset int // offset of signature in the original stream
	padExtra   []byte
}

// Align reads a raw ZIP from data and returns a new ZIP where every STORED entry's
// data starts at an offset that is a multiple of alignment.
// The Central Directory is updated to reflect any offset changes.
// Compressed (DEFLATE) entries are not padded.
func Align(data []byte, alignment int) ([]byte, error) {
	if alignment <= 0 {
		alignment = DefaultAlignment
	}

	// ---- Pass 1: parse all local entries ----
	entries, cdStart, err := parseLocalEntries(data)
	if err != nil {
		return nil, err
	}

	// ---- Pass 2: compute padding for each STORED entry ----
	// We simulate writing entries in order to know the exact offset of each entry's data.
	newOffsets := make([]int, len(entries)) // new local header offset for each entry
	pos := 0
	for i, e := range entries {
		newOffsets[i] = pos
		// Size contributed by this entry's local header + data
		localHdrSize := 30 + len(e.name) + len(e.extra)
		dataStart := pos + localHdrSize

		var padSize int
		if e.method == 0 { // STORED
			rem := dataStart % alignment
			if rem != 0 {
				padSize = alignment - rem
			}
		}
		entries[i].padExtra = make([]byte, padSize)
		pos += 30 + len(e.name) + len(e.extra) + padSize + len(e.data)
	}

	// ---- Pass 3: write new local entries ----
	var out bytes.Buffer
	for _, e := range entries {
		lhdr := make([]byte, 30)
		binary.LittleEndian.PutUint32(lhdr[0:], localFileSig)
		binary.LittleEndian.PutUint16(lhdr[4:], e.versionNeeded)
		binary.LittleEndian.PutUint16(lhdr[6:], e.flags)
		binary.LittleEndian.PutUint16(lhdr[8:], e.method)
		binary.LittleEndian.PutUint16(lhdr[10:], e.modTime)
		binary.LittleEndian.PutUint16(lhdr[12:], e.modDate)
		binary.LittleEndian.PutUint32(lhdr[14:], e.crc32)
		binary.LittleEndian.PutUint32(lhdr[18:], e.compressedSize)
		binary.LittleEndian.PutUint32(lhdr[22:], e.uncompSize)
		binary.LittleEndian.PutUint16(lhdr[26:], uint16(len(e.name)))
		binary.LittleEndian.PutUint16(lhdr[28:], uint16(len(e.extra)+len(e.padExtra)))
		out.Write(lhdr)
		out.Write(e.name)
		out.Write(e.extra)
		out.Write(e.padExtra)
		out.Write(e.data)
	}

	// ---- Pass 4: parse and rewrite Central Directory with updated offsets ----
	cdData := data[cdStart:]
	newCD, eocdData, err := rewriteCD(cdData, entries, newOffsets)
	if err != nil {
		return nil, err
	}

	newCDStart := out.Len()
	out.Write(newCD)

	// ---- Pass 5: rewrite EOCD with updated CD offset and size ----
	newEOCD := make([]byte, len(eocdData))
	copy(newEOCD, eocdData)
	binary.LittleEndian.PutUint32(newEOCD[12:], uint32(len(newCD)))
	binary.LittleEndian.PutUint32(newEOCD[16:], uint32(newCDStart))
	out.Write(newEOCD)

	return out.Bytes(), nil
}

// parseLocalEntries reads all local file entries from the ZIP data.
// Returns the entries and the offset where the Central Directory begins.
func parseLocalEntries(data []byte) ([]localEntry, int, error) {
	r := bytes.NewReader(data)
	var entries []localEntry

	for {
		pos := int(r.Size()) - r.Len()
		var sig uint32
		if err := binary.Read(r, binary.LittleEndian, &sig); err != nil {
			if err == io.EOF {
				break
			}
			return nil, 0, fmt.Errorf("reading signature at %d: %w", pos, err)
		}

		switch sig {
		case localFileSig:
			e, err := readLocalEntry(r)
			if err != nil {
				return nil, 0, fmt.Errorf("reading local entry at %d: %w", pos, err)
			}
			e.origOffset = pos
			entries = append(entries, e)

		case centralDirSig, eocdSig:
			// Back up 4 bytes so we return the CD start correctly
			r.Seek(-4, io.SeekCurrent)
			cdStart := int(r.Size()) - r.Len()
			return entries, cdStart, nil

		default:
			return nil, 0, fmt.Errorf("unexpected signature 0x%08x at offset %d", sig, pos)
		}
	}
	return entries, int(r.Size()), nil
}

func readLocalEntry(r *bytes.Reader) (localEntry, error) {
	const fixedHdrSize = 26
	hdr := make([]byte, fixedHdrSize)
	if _, err := io.ReadFull(r, hdr); err != nil {
		return localEntry{}, err
	}

	var e localEntry
	e.versionNeeded = binary.LittleEndian.Uint16(hdr[0:])
	e.flags = binary.LittleEndian.Uint16(hdr[2:])
	e.method = binary.LittleEndian.Uint16(hdr[4:])
	e.modTime = binary.LittleEndian.Uint16(hdr[6:])
	e.modDate = binary.LittleEndian.Uint16(hdr[8:])
	e.crc32 = binary.LittleEndian.Uint32(hdr[10:])
	e.compressedSize = binary.LittleEndian.Uint32(hdr[14:])
	e.uncompSize = binary.LittleEndian.Uint32(hdr[18:])
	nameLen := binary.LittleEndian.Uint16(hdr[22:])
	extraLen := binary.LittleEndian.Uint16(hdr[24:])

	e.name = make([]byte, nameLen)
	if _, err := io.ReadFull(r, e.name); err != nil {
		return e, fmt.Errorf("reading name: %w", err)
	}
	e.extra = make([]byte, extraLen)
	if _, err := io.ReadFull(r, e.extra); err != nil {
		return e, fmt.Errorf("reading extra: %w", err)
	}
	e.data = make([]byte, e.compressedSize)
	if _, err := io.ReadFull(r, e.data); err != nil {
		return e, fmt.Errorf("reading data (%d bytes): %w", e.compressedSize, err)
	}
	return e, nil
}

// rewriteCD rewrites CD entries with updated local header offsets.
// It parses CD entries from cdData (which begins at the CD in the original file),
// updates the offset of each local file header based on newOffsets, and returns
// the new CD bytes and the EOCD bytes.
func rewriteCD(cdData []byte, entries []localEntry, newOffsets []int) ([]byte, []byte, error) {
	// Build a name → new offset map for fast lookup
	nameToNewOffset := make(map[string]int, len(entries))
	for i, e := range entries {
		nameToNewOffset[string(e.name)] = newOffsets[i]
	}

	r := bytes.NewReader(cdData)
	var cdOut bytes.Buffer
	var eocdBytes []byte

	for {
		pos := int(r.Size()) - r.Len()
		var sig uint32
		if err := binary.Read(r, binary.LittleEndian, &sig); err != nil {
			if err == io.EOF {
				break
			}
			return nil, nil, fmt.Errorf("CD: reading sig at %d: %w", pos, err)
		}

		switch sig {
		case centralDirSig:
			// CD entry fixed header: 42 bytes after signature
			const fixedCDSize = 42
			hdr := make([]byte, fixedCDSize)
			if _, err := io.ReadFull(r, hdr); err != nil {
				return nil, nil, fmt.Errorf("reading CD header: %w", err)
			}
			nameLen := binary.LittleEndian.Uint16(hdr[24:])
			extraLen := binary.LittleEndian.Uint16(hdr[26:])
			commentLen := binary.LittleEndian.Uint16(hdr[28:])

			name := make([]byte, nameLen)
			io.ReadFull(r, name)
			extra := make([]byte, extraLen)
			io.ReadFull(r, extra)
			comment := make([]byte, commentLen)
			io.ReadFull(r, comment)

			// Update local header offset (field at hdr[38:42])
			if newOff, ok := nameToNewOffset[string(name)]; ok {
				binary.LittleEndian.PutUint32(hdr[38:], uint32(newOff))
			}

			var sigBytes [4]byte
			binary.LittleEndian.PutUint32(sigBytes[:], sig)
			cdOut.Write(sigBytes[:])
			cdOut.Write(hdr)
			cdOut.Write(name)
			cdOut.Write(extra)
			cdOut.Write(comment)

		case eocdSig:
			// Read rest of EOCD
			rest, _ := io.ReadAll(r)
			eocdBytes = make([]byte, 4+len(rest))
			binary.LittleEndian.PutUint32(eocdBytes[0:], sig)
			copy(eocdBytes[4:], rest)
			return cdOut.Bytes(), eocdBytes, nil

		default:
			return nil, nil, fmt.Errorf("CD: unexpected signature 0x%08x at %d", sig, pos)
		}
	}

	return cdOut.Bytes(), eocdBytes, nil
}
