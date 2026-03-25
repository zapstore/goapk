// Package res generates Android resources.arsc files.
//
// Format reference: AOSP frameworks/base/include/androidfw/ResourceTypes.h
//
// resources.arsc layout:
//
//	ResTable_header           (type=0x0002)
//	  ResStringPool_header    (global values string pool)
//	  ResTable_package        (type=0x0200)
//	    ResStringPool_header  (type-name string pool)
//	    ResStringPool_header  (key-name string pool)
//	    ResTable_typeSpec     (type=0x0202, one per resource type)
//	    ResTable_type         (type=0x0201, one per type+config combination)
package res

import "encoding/binary"

// Resource type indices within our app package (1-indexed in the type pool).
const (
	typeString = uint8(1) // res type: string (type pool idx 0 → ID type bits 0x01)
	typeMipmap = uint8(2) // res type: mipmap (type pool idx 1 → ID type bits 0x02)
)

// Entry indices for each type (0-indexed).
const (
	entryAppName   = uint16(0) // @string/app_name   → 0x7f010000
	entryIconColor = uint16(0) // @mipmap/ic_launcher → 0x7f020000
	entryIconMono  = uint16(1) // @mipmap/ic_launcher_mono → 0x7f020001
)

// ResConfig represents an Android configuration (density bucket, etc.).
// We use a simplified version with only the density field set.
type ResConfig struct {
	density uint16 // Android density value
	name    string // human-readable name for the dir suffix
}

// Standard Android density values (from ResTable_config).
var densityConfigs = []ResConfig{
	{0x00A0, "mdpi"},    // 160 dpi
	{0x00F0, "hdpi"},    // 240 dpi
	{0x0140, "xhdpi"},   // 320 dpi
	{0x01E0, "xxhdpi"},  // 480 dpi
	{0x0280, "xxxhdpi"}, // 640 dpi
}

// Params holds all inputs needed to generate resources.arsc.
type Params struct {
	AppName    string   // app display name string
	PkgName    string   // Java package name (for the res table package)
	IconPaths  []string // color icon paths ordered mdpi→xxxhdpi (len must match densityConfigs)
	MonoPaths  []string // monochrome icon paths; may be nil to omit mono icons
}

// Encode produces the binary resources.arsc for a minimal WebView app.
// It includes:
//   - @string/app_name
//   - @mipmap/ic_launcher (5 densities)
//   - @mipmap/ic_launcher_mono (5 densities, if MonoPaths non-nil)
func Encode(p Params) []byte {
	hasMono := len(p.MonoPaths) == len(densityConfigs)

	// ---- Global string pool ----
	// Contains the file paths for mipmap entries and the app name value.
	gsp := newSP()
	gsp.intern("") // entry 0: empty (used for null references)
	for _, path := range p.IconPaths {
		gsp.intern(apkPath(path, "ic_launcher"))
	}
	if hasMono {
		for _, path := range p.MonoPaths {
			gsp.intern(apkPath(path, "ic_launcher_mono"))
		}
	}
	// App name string value
	appNameIdx := gsp.intern(p.AppName)
	_ = appNameIdx // used below in string entry

	gspChunk := encodeStringPool(gsp)

	// ---- Package chunk ----
	pkgChunk := encodePackage(p, gsp, hasMono)

	// ---- ResTable_header ----
	// type(2) headerSize(2) size(4) packageCount(4)
	body := append(gspChunk, pkgChunk...)
	hdr := make([]byte, 12)
	binary.LittleEndian.PutUint16(hdr[0:], 0x0002) // RES_TABLE_TYPE
	binary.LittleEndian.PutUint16(hdr[2:], 12)     // headerSize
	binary.LittleEndian.PutUint32(hdr[4:], uint32(12+len(body)))
	binary.LittleEndian.PutUint32(hdr[8:], 1) // packageCount

	return append(hdr, body...)
}

// apkPath returns the APK-internal path for a mipmap entry.
// The caller (build package) already provides the correct APK-relative path
// (e.g. "res/mipmap-mdpi-v4/ic_launcher.png"), so we return it as-is.
func apkPath(filePath, _ string) string {
	return filePath
}

// encodePackage encodes the ResTable_package chunk for app package 0x7f.
func encodePackage(p Params, gsp *strPool, hasMono bool) []byte {
	// Type-name string pool: "string", "mipmap"
	tsp := newSP()
	tsp.intern("string") // index 0
	tsp.intern("mipmap") // index 1
	tspChunk := encodeStringPool(tsp)

	// Key-name string pool: "app_name", "ic_launcher", optionally "ic_launcher_mono"
	ksp := newSP()
	ksp.intern("app_name")     // index 0
	ksp.intern("ic_launcher")  // index 1
	if hasMono {
		ksp.intern("ic_launcher_mono") // index 2
	}
	kspChunk := encodeStringPool(ksp)

	// Number of mipmap entries
	mipmapCount := 1
	if hasMono {
		mipmapCount = 2
	}

	// ResTable_typeSpec for string type: 1 entry
	strSpecChunk := encodeTypeSpec(typeString, 1)
	// ResTable_typeSpec for mipmap type: 1 or 2 entries
	mipmapSpecChunk := encodeTypeSpec(typeMipmap, uint32(mipmapCount))

	// ResTable_type for string (default config): app_name value
	strTypeChunk := encodeStringType(p.AppName, gsp)

	// ResTable_type entries for mipmap per density
	var mipmapTypeChunks []byte
	for ci, cfg := range densityConfigs {
		paths := []string{p.IconPaths[ci]}
		if hasMono {
			paths = append(paths, p.MonoPaths[ci])
		}
		chunk := encodeMipmapType(cfg, paths, gsp)
		mipmapTypeChunks = append(mipmapTypeChunks, chunk...)
	}

	// Assemble package body (without the package header itself)
	var pkgBody []byte
	pkgBody = append(pkgBody, tspChunk...)
	pkgBody = append(pkgBody, kspChunk...)
	pkgBody = append(pkgBody, strSpecChunk...)
	pkgBody = append(pkgBody, strTypeChunk...)
	pkgBody = append(pkgBody, mipmapSpecChunk...)
	pkgBody = append(pkgBody, mipmapTypeChunks...)

	// ResTable_package header: type(2) headerSize(2) size(4) id(4) name(256) typeStrings(4)
	//   lastPublicType(4) keyStrings(4) lastPublicKey(4) typeIdOffset(4)
	// headerSize = 288 bytes
	const pkgHeaderSize = 288
	pkgHeader := make([]byte, pkgHeaderSize)
	binary.LittleEndian.PutUint16(pkgHeader[0:], 0x0200) // RES_TABLE_PACKAGE_TYPE
	binary.LittleEndian.PutUint16(pkgHeader[2:], pkgHeaderSize)
	binary.LittleEndian.PutUint32(pkgHeader[4:], uint32(pkgHeaderSize+len(pkgBody)))
	binary.LittleEndian.PutUint32(pkgHeader[8:], 0x7f) // package id

	// Package name (null-terminated UTF-16LE in 256-byte field at offset 12)
	nameRunes := []rune(p.PkgName)
	for i, r := range nameRunes {
		if 12+i*2+1 >= pkgHeaderSize {
			break
		}
		binary.LittleEndian.PutUint16(pkgHeader[12+i*2:], uint16(r))
	}

	// typeStrings offset: relative to start of package chunk (header + body)
	typeStringsOff := uint32(pkgHeaderSize) // immediately after header
	binary.LittleEndian.PutUint32(pkgHeader[268:], typeStringsOff)
	binary.LittleEndian.PutUint32(pkgHeader[272:], 2) // lastPublicType

	keyStringsOff := typeStringsOff + uint32(len(tspChunk))
	binary.LittleEndian.PutUint32(pkgHeader[276:], keyStringsOff)
	keyCount := uint32(2)
	if hasMono {
		keyCount = 3
	}
	binary.LittleEndian.PutUint32(pkgHeader[280:], keyCount) // lastPublicKey

	return append(pkgHeader, pkgBody...)
}

// encodeTypeSpec encodes a ResTable_typeSpec chunk.
// specFlags for each entry: 0 = no config-specific variants known (simplified).
func encodeTypeSpec(typeID uint8, entryCount uint32) []byte {
	headerSize := uint32(8)
	totalSize := headerSize + entryCount*4
	hdr := make([]byte, 8)
	binary.LittleEndian.PutUint16(hdr[0:], 0x0202) // RES_TABLE_TYPE_SPEC_TYPE
	binary.LittleEndian.PutUint16(hdr[2:], 8)
	binary.LittleEndian.PutUint32(hdr[4:], totalSize)
	// id(1) res0(1) res1(2) entryCount(4)
	// Unfortunately the header for typeSpec is:
	// type(2) headerSize(2) size(4) id(1) res0(1) res1(2) entryCount(4)  = 16 bytes
	// Let me correct this.
	hdr = make([]byte, 16)
	binary.LittleEndian.PutUint16(hdr[0:], 0x0202)
	binary.LittleEndian.PutUint16(hdr[2:], 16) // headerSize
	binary.LittleEndian.PutUint32(hdr[4:], 16+entryCount*4)
	hdr[8] = typeID // id (1-indexed)
	// res0=0, res1=0
	binary.LittleEndian.PutUint32(hdr[12:], entryCount)

	var flags []byte
	for i := uint32(0); i < entryCount; i++ {
		tmp := make([]byte, 4)
		// FLAG_PUBLIC = 0x40000000; mark all entries as public
		binary.LittleEndian.PutUint32(tmp, 0x40000000)
		flags = append(flags, tmp...)
	}
	return append(hdr, flags...)
}

// encodeStringType encodes a ResTable_type for the "string" type with the default config.
// Contains one entry: app_name = the app display name.
func encodeStringType(appName string, gsp *strPool) []byte {
	return encodeType(typeString, ResConfig{}, []stringEntry{
		{key: 0, strIdx: uint32(gsp.idx(appName))},
	})
}

type stringEntry struct {
	key    uint32 // key index in the key string pool
	strIdx uint32 // index into the global string pool
}

// encodeMipmapType encodes a ResTable_type for the "mipmap" type for one density config.
func encodeMipmapType(cfg ResConfig, paths []string, gsp *strPool) []byte {
	entries := make([]stringEntry, len(paths))
	for i, path := range paths {
		keyName := "ic_launcher"
		if i == 1 {
			keyName = "ic_launcher_mono"
		}
		// path stored as file reference in the APK
		apkp := apkPath(path, keyName)
		entries[i] = stringEntry{key: uint32(i + 1), strIdx: uint32(gsp.idx(apkp))}
	}
	return encodeType(typeMipmap, cfg, entries)
}

// encodeType encodes a ResTable_type chunk.
// Simplified: only handles string-type entries (file paths in the global string pool).
//
// ResTable_type layout:
//
//	header (52 bytes): type(2) headerSize(2) size(4) id(1) flags(1) reserved(2)
//	                   entryCount(4) entriesStart(4) config(32)
//	entry offsets: entryCount × uint32
//	entries: ResTable_entry + ResTable_value per entry
func encodeType(typeID uint8, cfg ResConfig, entries []stringEntry) []byte {
	const headerSize = 52
	const configSize = 32
	entryCount := uint32(len(entries))

	// Build entry offsets (relative to entriesStart)
	offsets := make([]uint32, entryCount)
	const entrySize = 16 // ResTable_entry(8) + ResTable_value(8)
	for i := range entries {
		offsets[i] = uint32(i) * entrySize
	}

	entriesStart := uint32(headerSize) + entryCount*4
	totalSize := entriesStart + entryCount*entrySize

	hdr := make([]byte, headerSize)
	binary.LittleEndian.PutUint16(hdr[0:], 0x0201) // RES_TABLE_TYPE_TYPE
	binary.LittleEndian.PutUint16(hdr[2:], headerSize)
	binary.LittleEndian.PutUint32(hdr[4:], totalSize)
	hdr[8] = typeID // id (1-indexed, matches typeSpec)
	// flags=0, reserved=0
	binary.LittleEndian.PutUint32(hdr[12:], entryCount)
	binary.LittleEndian.PutUint32(hdr[16:], entriesStart)

	// Config struct (32 bytes) at offset 20 within the type header.
	// ResTable_config layout: size(4) mcc(2) mnc(2) lang(2) country(2)
	//   orientation(1) touchscreen(1) density(2) ...
	// Density is at offset 14 within ResTable_config.
	config := make([]byte, configSize)
	binary.LittleEndian.PutUint32(config[0:], configSize)   // size
	binary.LittleEndian.PutUint16(config[14:], cfg.density) // density
	copy(hdr[20:], config)

	var body []byte
	// Offset table
	for _, off := range offsets {
		tmp := make([]byte, 4)
		binary.LittleEndian.PutUint32(tmp, off)
		body = append(body, tmp...)
	}
	// Entries: ResTable_entry(8) + ResTable_value(8)
	for _, e := range entries {
		// ResTable_entry: size(2) flags(1) key(4) → padded to 8 bytes
		entry := make([]byte, 8)
		binary.LittleEndian.PutUint16(entry[0:], 8) // size
		// flags=0 (simple value)
		binary.LittleEndian.PutUint32(entry[4:], e.key) // key index
		// Hmm, the entry format is: size(2) flags(2) key(4) = 8 bytes
		entry = make([]byte, 8)
		binary.LittleEndian.PutUint16(entry[0:], 8)    // size
		binary.LittleEndian.PutUint16(entry[2:], 0)    // flags
		binary.LittleEndian.PutUint32(entry[4:], e.key) // key

		// ResTable_value: size(2) res0(1) dataType(1) data(4)
		val := make([]byte, 8)
		binary.LittleEndian.PutUint16(val[0:], 8)    // size
		val[2] = 0                                   // res0
		val[3] = 0x03                                // TYPE_STRING
		binary.LittleEndian.PutUint32(val[4:], e.strIdx)

		body = append(body, entry...)
		body = append(body, val...)
	}
	return append(hdr, body...)
}

// ---- String pool (shared with xmlbin, duplicated here to avoid circular imports) ----

type strPool struct {
	strings []string
	index   map[string]int
}

func newSP() *strPool {
	return &strPool{index: map[string]int{}}
}

func (sp *strPool) intern(s string) int {
	if i, ok := sp.index[s]; ok {
		return i
	}
	i := len(sp.strings)
	sp.strings = append(sp.strings, s)
	sp.index[s] = i
	return i
}

func (sp *strPool) idx(s string) int {
	if i, ok := sp.index[s]; ok {
		return i
	}
	return sp.intern(s)
}

// encodeStringPool encodes a ResStringPool_header chunk using UTF-16LE strings.
func encodeStringPool(sp *strPool) []byte {
	type enc struct{ data []byte }
	encoded := make([]enc, len(sp.strings))
	for i, s := range sp.strings {
		runes := []rune(s)
		n := len(runes)
		buf := make([]byte, 2+n*2+2)
		binary.LittleEndian.PutUint16(buf[0:], uint16(n))
		for j, r := range runes {
			binary.LittleEndian.PutUint16(buf[2+j*2:], uint16(r))
		}
		encoded[i] = enc{buf}
	}

	offsets := make([]uint32, len(encoded))
	var strData []byte
	for i, e := range encoded {
		offsets[i] = uint32(len(strData))
		strData = append(strData, e.data...)
	}
	for len(strData)%4 != 0 {
		strData = append(strData, 0)
	}

	const headerSize = uint32(28)
	offsetsSize := uint32(len(offsets) * 4)
	stringsStart := headerSize + offsetsSize
	totalSize := stringsStart + uint32(len(strData))

	hdr := make([]byte, headerSize)
	binary.LittleEndian.PutUint16(hdr[0:], 0x0001) // RES_STRING_POOL_TYPE
	binary.LittleEndian.PutUint16(hdr[2:], uint16(headerSize))
	binary.LittleEndian.PutUint32(hdr[4:], totalSize)
	binary.LittleEndian.PutUint32(hdr[8:], uint32(len(sp.strings)))
	binary.LittleEndian.PutUint32(hdr[20:], stringsStart)

	var out []byte
	out = append(out, hdr...)
	for _, off := range offsets {
		tmp := make([]byte, 4)
		binary.LittleEndian.PutUint32(tmp, off)
		out = append(out, tmp...)
	}
	out = append(out, strData...)
	return out
}
