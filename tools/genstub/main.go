// genstub generates a minimal valid DEX file containing no classes.
// The output is a structurally correct DEX 035 with only a header and map list.
// It can be used as a placeholder when the real classes.dex has not been compiled yet.
//
// Usage: go run ./tools/genstub [-o path]
package main

import (
	"crypto/sha1"
	"encoding/binary"
	"flag"
	"fmt"
	"hash/adler32"
	"os"
)

// DEX 035 empty file layout:
//
//	[0x00:0x70]  Header (112 bytes)
//	[0x70:0x8C]  Map list (28 bytes: 4-byte count + 2 × 12-byte entries)
//
// Total: 140 bytes (0x8C)
const fileSize = 0x8C

func main() {
	out := flag.String("o", "internal/embed/classes.dex", "output path")
	flag.Parse()

	buf := make([]byte, fileSize)

	// magic: "dex\n035\0"
	copy(buf[0:], "dex\n035\x00")

	// file_size (LE uint32)
	binary.LittleEndian.PutUint32(buf[32:], fileSize)
	// header_size = 0x70
	binary.LittleEndian.PutUint32(buf[36:], 0x70)
	// endian_tag = 0x12345678
	binary.LittleEndian.PutUint32(buf[40:], 0x12345678)
	// map_off = 0x70 (immediately after header)
	binary.LittleEndian.PutUint32(buf[52:], 0x70)

	// Map list at offset 0x70
	// size = 2 entries
	binary.LittleEndian.PutUint32(buf[0x70:], 2)

	// Entry 0: TYPE_HEADER_ITEM (0x0000), count=1, offset=0
	binary.LittleEndian.PutUint16(buf[0x74:], 0x0000) // type
	binary.LittleEndian.PutUint16(buf[0x76:], 0)      // unused
	binary.LittleEndian.PutUint32(buf[0x78:], 1)      // size
	binary.LittleEndian.PutUint32(buf[0x7C:], 0)      // offset

	// Entry 1: TYPE_MAP_LIST (0x1000), count=1, offset=0x70
	binary.LittleEndian.PutUint16(buf[0x80:], 0x1000) // type
	binary.LittleEndian.PutUint16(buf[0x82:], 0)      // unused
	binary.LittleEndian.PutUint32(buf[0x84:], 1)      // size
	binary.LittleEndian.PutUint32(buf[0x88:], 0x70)   // offset

	// SHA-1 of bytes[32:end] — stored at bytes[12:32]
	sha := sha1.Sum(buf[32:])
	copy(buf[12:], sha[:])

	// Adler-32 of bytes[12:end] — stored at bytes[8:12]
	a32 := adler32.Checksum(buf[12:])
	binary.LittleEndian.PutUint32(buf[8:], a32)

	if err := os.WriteFile(*out, buf, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("wrote %d-byte DEX stub to %s\n", len(buf), *out)
}
