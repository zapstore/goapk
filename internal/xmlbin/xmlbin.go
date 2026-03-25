// Package xmlbin encodes XML documents into Android's binary XML format (AXML).
//
// Format reference: AOSP frameworks/base/include/androidfw/ResourceTypes.h
// The binary XML format uses length-prefixed chunks. All integers are little-endian.
//
// Chunk layout for a manifest:
//
//	ResXMLTree_header       (file header, type=0x0003)
//	ResStringPool_header    (string pool, type=0x0001)
//	ResourceMap             (attr name → framework resource ID, type=0x0180)
//	ResXMLTree_node         (namespace start, type=0x0100)
//	ResXMLTree_node*        (elements and attributes, type=0x0102/0x0103)
//	ResXMLTree_node         (namespace end, type=0x0101)
package xmlbin

import (
	"encoding/binary"
	"strings"
)

// Chunk type codes.
const (
	chunkStringPool    = uint16(0x0001)
	chunkXML           = uint16(0x0003)
	chunkXMLStartNS    = uint16(0x0100)
	chunkXMLEndNS      = uint16(0x0101)
	chunkXMLStartElem  = uint16(0x0102)
	chunkXMLEndElem    = uint16(0x0103)
	chunkXMLResourceMap = uint16(0x0180)
)

// ResTable_value data types (used in attribute values).
const (
	TypeNull      = uint8(0x00)
	TypeReference = uint8(0x01) // resource reference (@0x7f...)
	TypeString    = uint8(0x03) // string pool index
	TypeIntDec    = uint8(0x10) // plain integer
	TypeIntHex    = uint8(0x11) // hex integer
	TypeIntBool   = uint8(0x12) // boolean: 0=false, 0xFFFFFFFF=true
)

// Android framework attribute resource IDs from AOSP DumpManifest.cpp.
const (
	AttrLabel            = uint32(0x01010001)
	AttrIcon             = uint32(0x01010002)
	AttrName             = uint32(0x01010003)
	AttrDebuggable       = uint32(0x0101000f)
	AttrExported         = uint32(0x01010010)
	AttrVersionCode      = uint32(0x0101021b)
	AttrVersionName      = uint32(0x0101021c)
	AttrMinSDKVersion    = uint32(0x0101020c)
	AttrTargetSDKVersion = uint32(0x01010270)
	AttrAllowBackup      = uint32(0x01010280)
)

// androidNS is the standard Android attribute namespace URI.
const androidNS = "http://schemas.android.com/apk/res/android"

// Value represents a typed attribute value.
type Value struct {
	DataType uint8
	Data     uint32
}

// RefVal returns a resource reference value.
func RefVal(id uint32) Value { return Value{TypeReference, id} }

// StrVal returns a string value (data = string pool index, set by encoder).
func StrVal(s string) Value { return Value{TypeString, 0} } // index filled during encode

// IntVal returns a decimal integer value.
func IntVal(n int32) Value { return Value{TypeIntDec, uint32(n)} }

// HexVal returns a hex integer value.
func HexVal(n uint32) Value { return Value{TypeIntHex, n} }

// BoolVal returns a boolean value.
func BoolVal(b bool) Value {
	if b {
		return Value{TypeIntBool, 0xFFFFFFFF}
	}
	return Value{TypeIntBool, 0}
}

// Attr is a single XML attribute.
type Attr struct {
	NS    string // namespace URI, or "" for no-namespace attrs
	Name  string // local name
	ResID uint32 // Android framework resource ID (0 if not a framework attr)
	Val   Value
	// strVal is the string content when Val.DataType == TypeString
	strVal string
}

// StrAttr creates a no-namespace string attribute (e.g. "package").
func StrAttr(name, val string) Attr {
	return Attr{NS: "", Name: name, ResID: 0, Val: Value{TypeString, 0}, strVal: val}
}

// AndroidAttr creates an android: namespace attribute with a framework resource ID.
func AndroidAttr(name string, resID uint32, val Value) Attr {
	a := Attr{NS: androidNS, Name: name, ResID: resID, Val: val}
	if val.DataType == TypeString {
		panic("use AndroidStrAttr for string values")
	}
	return a
}

// AndroidStrAttr creates an android: namespace attribute with a string value.
func AndroidStrAttr(name string, resID uint32, strVal string) Attr {
	return Attr{NS: androidNS, Name: name, ResID: resID, Val: Value{TypeString, 0}, strVal: strVal}
}

// event represents a single XML event (namespace, element, or end).
type event struct {
	kind   eventKind
	ns     string
	name   string
	attrs  []Attr
	lineNo uint32
}

type eventKind int

const (
	evStartNS  eventKind = iota
	evEndNS
	evStartElem
	evEndElem
)

// Encoder accumulates XML events and encodes them to Android binary XML.
type Encoder struct {
	events []event
}

// NewEncoder creates a new Encoder.
func NewEncoder() *Encoder {
	return &Encoder{}
}

// StartNamespace records a namespace declaration.
func (e *Encoder) StartNamespace(prefix, uri string) {
	e.events = append(e.events, event{kind: evStartNS, ns: prefix, name: uri})
}

// EndNamespace records the end of a namespace.
func (e *Encoder) EndNamespace(prefix, uri string) {
	e.events = append(e.events, event{kind: evEndNS, ns: prefix, name: uri})
}

// StartElement records the start of an element with the given attributes.
func (e *Encoder) StartElement(ns, name string, attrs []Attr) {
	e.events = append(e.events, event{kind: evStartElem, ns: ns, name: name, attrs: attrs, lineNo: 1})
}

// EndElement records the end of an element.
func (e *Encoder) EndElement(ns, name string) {
	e.events = append(e.events, event{kind: evEndElem, ns: ns, name: name, lineNo: 1})
}

// Encode produces the binary XML bytes.
func (e *Encoder) Encode() []byte {
	// ---- Pass 1: collect all strings ----
	sp := newStringPool()

	// Pre-intern namespace strings
	sp.intern("") // empty string at index 0 for "no namespace" attributes
	sp.intern("android")
	sp.intern(androidNS)

	// Intern all element names and attribute names/values
	for _, ev := range e.events {
		if ev.ns != "" && ev.ns != androidNS {
			sp.intern(ev.ns)
		}
		sp.intern(ev.name)
		for _, a := range ev.attrs {
			if a.NS != "" {
				sp.intern(a.NS)
			}
			sp.intern(a.Name)
			if a.Val.DataType == TypeString {
				sp.intern(a.strVal)
			}
		}
	}

	// ---- Pass 2: build resource map (one entry per string pool string) ----
	// We need to map attribute names to their framework resource IDs.
	// Build a name→resID lookup from all attrs.
	nameToResID := map[string]uint32{}
	for _, ev := range e.events {
		for _, a := range ev.attrs {
			if a.ResID != 0 {
				nameToResID[a.Name] = a.ResID
			}
		}
	}

	// Resource map has one entry per string pool entry (0 for non-attribute strings).
	resMap := make([]uint32, sp.count())
	for i, s := range sp.strings {
		if resID, ok := nameToResID[s]; ok {
			resMap[i] = resID
		}
	}

	// ---- Pass 3: encode all chunks ----
	var body []byte

	// Resource map chunk
	rmChunk := encodeResMap(resMap)
	body = append(body, rmChunk...)

	// XML events
	for _, ev := range e.events {
		switch ev.kind {
		case evStartNS:
			body = append(body, encodeNS(chunkXMLStartNS, sp, ev.ns, ev.name, ev.lineNo)...)
		case evEndNS:
			body = append(body, encodeNS(chunkXMLEndNS, sp, ev.ns, ev.name, ev.lineNo)...)
		case evStartElem:
			body = append(body, encodeStartElem(sp, ev, nameToResID)...)
		case evEndElem:
			body = append(body, encodeEndElem(sp, ev)...)
		}
	}

	// String pool chunk (must come before body in the output, but we build it after)
	spChunk := encodeStringPool(sp)

	// Assemble file: file header + string pool + resource map + events
	fileBody := append(spChunk, body...)

	fileHeader := make([]byte, 8)
	binary.LittleEndian.PutUint16(fileHeader[0:], chunkXML)
	binary.LittleEndian.PutUint16(fileHeader[2:], 8) // headerSize
	binary.LittleEndian.PutUint32(fileHeader[4:], uint32(8+len(fileBody)))

	return append(fileHeader, fileBody...)
}

// ---- String Pool ----

type stringPool struct {
	strings []string
	index   map[string]int
}

func newStringPool() *stringPool {
	return &stringPool{index: map[string]int{}}
}

// intern adds s to the pool if not present and returns its index.
func (sp *stringPool) intern(s string) int {
	if i, ok := sp.index[s]; ok {
		return i
	}
	i := len(sp.strings)
	sp.strings = append(sp.strings, s)
	sp.index[s] = i
	return i
}

func (sp *stringPool) idx(s string) int {
	if i, ok := sp.index[s]; ok {
		return i
	}
	return sp.intern(s)
}

func (sp *stringPool) count() int { return len(sp.strings) }

// encodeStringPool encodes the string pool chunk using UTF-16LE strings.
// Format: ResStringPool_header + string offsets + string data (UTF-16LE)
func encodeStringPool(sp *stringPool) []byte {
	// Encode each string as UTF-16LE: uint16 length + uint16 chars + uint16 null
	type encodedStr struct {
		data []byte
	}
	encoded := make([]encodedStr, len(sp.strings))
	for i, s := range sp.strings {
		runes := []rune(s)
		n := len(runes)
		buf := make([]byte, 2+n*2+2) // length(2) + chars(n*2) + null(2)
		binary.LittleEndian.PutUint16(buf[0:], uint16(n))
		for j, r := range runes {
			binary.LittleEndian.PutUint16(buf[2+j*2:], uint16(r))
		}
		// null terminator already zero from make
		encoded[i] = encodedStr{buf}
	}

	// Build offset table and string data
	offsets := make([]uint32, len(encoded))
	var strData []byte
	for i, e := range encoded {
		offsets[i] = uint32(len(strData))
		strData = append(strData, e.data...)
	}

	// Pad string data to 4-byte boundary
	for len(strData)%4 != 0 {
		strData = append(strData, 0)
	}

	// Header: type(2) headerSize(2) size(4) stringCount(4) styleCount(4)
	//         flags(4) stringsStart(4) stylesStart(4)
	headerSize := uint32(28)
	offsetsSize := uint32(len(offsets) * 4)
	stringsStart := headerSize + offsetsSize
	totalSize := stringsStart + uint32(len(strData))

	hdr := make([]byte, headerSize)
	binary.LittleEndian.PutUint16(hdr[0:], chunkStringPool)
	binary.LittleEndian.PutUint16(hdr[2:], uint16(headerSize))
	binary.LittleEndian.PutUint32(hdr[4:], totalSize)
	binary.LittleEndian.PutUint32(hdr[8:], uint32(len(sp.strings))) // stringCount
	// styleCount=0, flags=0, stringsStart, stylesStart=0
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

// ---- Resource Map ----

func encodeResMap(resMap []uint32) []byte {
	headerSize := uint32(8)
	totalSize := headerSize + uint32(len(resMap)*4)
	hdr := make([]byte, 8)
	binary.LittleEndian.PutUint16(hdr[0:], chunkXMLResourceMap)
	binary.LittleEndian.PutUint16(hdr[2:], 8) // headerSize
	binary.LittleEndian.PutUint32(hdr[4:], totalSize)

	var out []byte
	out = append(out, hdr...)
	for _, id := range resMap {
		tmp := make([]byte, 4)
		binary.LittleEndian.PutUint32(tmp, id)
		out = append(out, tmp...)
	}
	return out
}

// ---- Namespace nodes ----

// encodeNS encodes a ResXMLTree_namespaceExt node.
// For StartNS: ns=prefix, name=URI.
func encodeNS(chunkType uint16, sp *stringPool, prefix, uri string, lineNo uint32) []byte {
	// ResXMLTree_node: type(2) headerSize(2) size(4) lineNumber(4) comment(4)
	// ResXMLTree_namespaceExt: prefix(4) uri(4)
	hdr := make([]byte, 16)
	binary.LittleEndian.PutUint16(hdr[0:], chunkType)
	binary.LittleEndian.PutUint16(hdr[2:], 16)   // headerSize
	binary.LittleEndian.PutUint32(hdr[4:], 24)   // total size = 16 + 8
	binary.LittleEndian.PutUint32(hdr[8:], lineNo)
	binary.LittleEndian.PutUint32(hdr[12:], 0xFFFFFFFF) // comment = -1 (none)

	ext := make([]byte, 8)
	binary.LittleEndian.PutUint32(ext[0:], uint32(sp.idx(prefix)))
	binary.LittleEndian.PutUint32(ext[4:], uint32(sp.idx(uri)))
	return append(hdr, ext...)
}

// ---- Element nodes ----

// attrSize is the fixed byte size of a single encoded attribute.
// ResXMLTree_attribute: ns(4) name(4) rawValue(4) typedValue(8) = 20 bytes
const attrSize = 20

// encodeStartElem encodes a ResXMLTree_attrExt start-element node.
func encodeStartElem(sp *stringPool, ev event, nameToResID map[string]uint32) []byte {
	nAttrs := len(ev.attrs)

	// ResXMLTree_node (16) + ResXMLTree_attrExt header (20) + attrs (nAttrs*20)
	headerSize := uint32(16)
	extSize := uint32(20)
	attrsBytes := uint32(nAttrs * attrSize)
	totalSize := headerSize + extSize + attrsBytes

	buf := make([]byte, totalSize)

	// ResXMLTree_node
	binary.LittleEndian.PutUint16(buf[0:], chunkXMLStartElem)
	binary.LittleEndian.PutUint16(buf[2:], uint16(headerSize))
	binary.LittleEndian.PutUint32(buf[4:], totalSize)
	binary.LittleEndian.PutUint32(buf[8:], ev.lineNo)
	binary.LittleEndian.PutUint32(buf[12:], 0xFFFFFFFF) // comment

	// ResXMLTree_attrExt
	nsIdx := int32(-1)
	if ev.ns != "" {
		nsIdx = int32(sp.idx(ev.ns))
	}
	binary.LittleEndian.PutUint32(buf[16:], uint32(nsIdx))
	binary.LittleEndian.PutUint32(buf[20:], uint32(sp.idx(ev.name)))
	binary.LittleEndian.PutUint16(buf[24:], uint16(extSize)) // attributeStart
	binary.LittleEndian.PutUint16(buf[26:], attrSize)         // attributeSize
	binary.LittleEndian.PutUint16(buf[28:], uint16(nAttrs))   // attributeCount
	// idIndex, classIndex, styleIndex = 0

	// Encode attributes
	base := int(headerSize + extSize)
	for i, a := range ev.attrs {
		off := base + i*attrSize
		// ns index
		nsI := int32(-1)
		if a.NS != "" {
			nsI = int32(sp.idx(a.NS))
		}
		binary.LittleEndian.PutUint32(buf[off:], uint32(nsI))
		binary.LittleEndian.PutUint32(buf[off+4:], uint32(sp.idx(a.Name)))

		// rawValue: string index if string type, else -1
		if a.Val.DataType == TypeString {
			binary.LittleEndian.PutUint32(buf[off+8:], uint32(sp.idx(a.strVal)))
		} else {
			binary.LittleEndian.PutUint32(buf[off+8:], 0xFFFFFFFF)
		}

		// ResTable_value: size(2) res0(1) dataType(1) data(4)
		binary.LittleEndian.PutUint16(buf[off+12:], 8)          // size
		buf[off+14] = 0                                          // res0
		buf[off+15] = a.Val.DataType
		data := a.Val.Data
		if a.Val.DataType == TypeString {
			data = uint32(sp.idx(a.strVal))
		}
		binary.LittleEndian.PutUint32(buf[off+16:], data)
	}
	return buf
}

// encodeEndElem encodes a ResXMLTree_endElementExt end-element node.
func encodeEndElem(sp *stringPool, ev event) []byte {
	// ResXMLTree_node (16) + ResXMLTree_endElementExt (8)
	buf := make([]byte, 24)
	binary.LittleEndian.PutUint16(buf[0:], chunkXMLEndElem)
	binary.LittleEndian.PutUint16(buf[2:], 16) // headerSize
	binary.LittleEndian.PutUint32(buf[4:], 24) // totalSize
	binary.LittleEndian.PutUint32(buf[8:], ev.lineNo)
	binary.LittleEndian.PutUint32(buf[12:], 0xFFFFFFFF) // comment

	nsIdx := int32(-1)
	if ev.ns != "" {
		nsIdx = int32(sp.idx(ev.ns))
	}
	binary.LittleEndian.PutUint32(buf[16:], uint32(nsIdx))
	binary.LittleEndian.PutUint32(buf[20:], uint32(sp.idx(ev.name)))
	return buf
}

// ---- Convenience: AndroidManifest builder ----

// ManifestParams holds all the values needed to produce a binary AndroidManifest.xml.
type ManifestParams struct {
	Package       string
	VersionCode   int32
	VersionName   string
	MinSDK        int32
	TargetSDK     int32
	AppLabel      uint32 // resource ID for app name (@string/app_name)
	AppIcon       uint32 // resource ID for launcher icon (@mipmap/ic_launcher)
	ActivityClass string // fully-qualified class name
	Permissions   []string
}

// EncodeManifest produces the binary AndroidManifest.xml for a standard WebView wrapper.
func EncodeManifest(p ManifestParams) []byte {
	e := NewEncoder()
	e.StartNamespace("android", androidNS)

	// <manifest package="..." android:versionCode="..." android:versionName="...">
	e.StartElement("", "manifest", []Attr{
		StrAttr("package", p.Package),
		AndroidAttr("versionCode", AttrVersionCode, IntVal(p.VersionCode)),
		AndroidStrAttr("versionName", AttrVersionName, p.VersionName),
	})

	// <uses-sdk android:minSdkVersion="..." android:targetSdkVersion="..."/>
	e.StartElement("", "uses-sdk", []Attr{
		AndroidAttr("minSdkVersion", AttrMinSDKVersion, IntVal(p.MinSDK)),
		AndroidAttr("targetSdkVersion", AttrTargetSDKVersion, IntVal(p.TargetSDK)),
	})
	e.EndElement("", "uses-sdk")

	// <uses-permission android:name="..."/> for each permission
	for _, perm := range p.Permissions {
		e.StartElement("", "uses-permission", []Attr{
			AndroidStrAttr("name", AttrName, perm),
		})
		e.EndElement("", "uses-permission")
	}

	// <application android:label="@..." android:icon="@..." android:allowBackup="false">
	e.StartElement("", "application", []Attr{
		AndroidAttr("label", AttrLabel, RefVal(p.AppLabel)),
		AndroidAttr("icon", AttrIcon, RefVal(p.AppIcon)),
		AndroidAttr("allowBackup", AttrAllowBackup, BoolVal(false)),
	})

	// <activity android:name="..." android:exported="true">
	e.StartElement("", "activity", []Attr{
		AndroidStrAttr("name", AttrName, p.ActivityClass),
		AndroidAttr("exported", AttrExported, BoolVal(true)),
	})

	// <intent-filter>
	e.StartElement("", "intent-filter", nil)

	// <action android:name="android.intent.action.MAIN"/>
	e.StartElement("", "action", []Attr{
		AndroidStrAttr("name", AttrName, "android.intent.action.MAIN"),
	})
	e.EndElement("", "action")

	// <category android:name="android.intent.category.LAUNCHER"/>
	e.StartElement("", "category", []Attr{
		AndroidStrAttr("name", AttrName, "android.intent.category.LAUNCHER"),
	})
	e.EndElement("", "category")

	e.EndElement("", "intent-filter")
	e.EndElement("", "activity")
	e.EndElement("", "application")
	e.EndElement("", "manifest")
	e.EndNamespace("android", androidNS)

	return e.Encode()
}

// containsNS is used internally for building namespace-clean attribute names.
func containsNS(ns string) bool {
	return strings.Contains(ns, ":")
}
