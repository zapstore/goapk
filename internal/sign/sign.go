// Package sign implements APK Signature Scheme v2 signing and RSA keystore generation.
//
// Signing spec: https://source.android.com/docs/security/features/apksigning/v2
//
// The signing process:
//  1. Find the ZIP Central Directory (CD) and EOCD in the APK.
//  2. Compute digests of three sections: ZIP entries, CD, patched EOCD.
//  3. Build the signed-data block (digests + certificate chain).
//  4. Sign the signed-data block with RSA-PKCS1v15-SHA256.
//  5. Assemble the APK signing block.
//  6. Insert the signing block between the ZIP entries and the CD.
//  7. Patch the EOCD to update the CD offset.
package sign

import (
	"bytes"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/binary"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"time"

	"software.sslmate.com/src/go-pkcs12"
)

const (
	// APKSigningBlockMagic is the 16-byte magic that ends every APK signing block.
	APKSigningBlockMagic = "APK Sig Block 42"
	apkSigningBlockMagic = APKSigningBlockMagic // internal alias

	v2BlockID = uint32(0x7109871a)

	// Signature algorithm ID: RSASSA-PKCS1-v1_5 with SHA2-256 (deterministic)
	sigAlgRSAPKCS1SHA256 = uint32(0x0103)
)

// Keystore holds an RSA private key and its X.509 certificate.
type Keystore struct {
	PrivKey *rsa.PrivateKey
	Cert    *x509.Certificate
}

// LoadKeystore reads a PKCS12 keystore from path using the given password.
func LoadKeystore(path, password string) (*Keystore, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading keystore: %w", err)
	}
	privKey, cert, err := pkcs12.Decode(data, password)
	if err != nil {
		return nil, fmt.Errorf("decoding keystore: %w", err)
	}
	rk, ok := privKey.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("keystore must contain an RSA private key")
	}
	return &Keystore{PrivKey: rk, Cert: cert}, nil
}

// GenerateKeystore creates a new 2048-bit RSA debug keystore valid for 30 years
// and saves it to path as a PKCS12 file.
func GenerateKeystore(path, cn, password string) (*Keystore, error) {
	privKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, fmt.Errorf("generating RSA key: %w", err)
	}

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, fmt.Errorf("generating serial: %w", err)
	}

	now := time.Now()
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: cn},
		NotBefore:    now,
		NotAfter:     now.AddDate(30, 0, 0),
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}
	certDER, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &privKey.PublicKey, privKey)
	if err != nil {
		return nil, fmt.Errorf("creating certificate: %w", err)
	}
	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		return nil, fmt.Errorf("parsing certificate: %w", err)
	}

	p12Data, err := pkcs12.Encode(rand.Reader, privKey, cert, nil, password)
	if err != nil {
		return nil, fmt.Errorf("encoding PKCS12: %w", err)
	}

	if err := os.WriteFile(path, p12Data, 0600); err != nil {
		return nil, fmt.Errorf("writing keystore: %w", err)
	}
	return &Keystore{PrivKey: privKey, Cert: cert}, nil
}

// GenerateDebugKeystore creates a temporary in-memory debug keystore without saving to disk.
func GenerateDebugKeystore() (*Keystore, error) {
	privKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, fmt.Errorf("generating debug key: %w", err)
	}
	serial, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	now := time.Now()
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: "Android Debug"},
		NotBefore:    now,
		NotAfter:     now.AddDate(30, 0, 0),
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}
	certDER, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &privKey.PublicKey, privKey)
	if err != nil {
		return nil, fmt.Errorf("creating debug certificate: %w", err)
	}
	cert, _ := x509.ParseCertificate(certDER)
	return &Keystore{PrivKey: privKey, Cert: cert}, nil
}

// ExportCertPEM returns the certificate as a PEM-encoded string (for display).
func (k *Keystore) ExportCertPEM() string {
	return string(pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: k.Cert.Raw,
	}))
}

// Sign applies APK Signature Scheme v2 to an aligned APK and returns the signed bytes.
func Sign(apkData []byte, ks *Keystore) ([]byte, error) {
	// Locate Central Directory and EOCD
	eocdOff, err := findEOCDOffset(apkData)
	if err != nil {
		return nil, fmt.Errorf("locating EOCD: %w", err)
	}
	if eocdOff < 22 {
		return nil, fmt.Errorf("EOCD offset too small")
	}

	cdOffset, cdSize, err := parseCDFromEOCD(apkData, eocdOff)
	if err != nil {
		return nil, fmt.Errorf("parsing CD from EOCD: %w", err)
	}

	// The three sections to digest:
	// Section 1: [0 : cdOffset]          (ZIP entries)
	// Section 3: [cdOffset : eocdOff]    (Central Directory)
	// Section 4: [eocdOff : end]         (EOCD, with CD offset field zeroed out)
	section1 := apkData[:cdOffset]
	section3 := apkData[cdOffset : cdOffset+cdSize]
	section4 := patchEOCDOffset(apkData[eocdOff:], uint32(cdOffset)) // patch will be overwritten later

	// Compute the content digest
	digest, err := computeAPKDigest(section1, section3, section4)
	if err != nil {
		return nil, fmt.Errorf("computing APK digest: %w", err)
	}

	// Build the signed-data block
	certDER := ks.Cert.Raw
	signedData := buildSignedData(digest, certDER)

	// Sign the signed-data block
	hashed := sha256.Sum256(signedData)
	sig, err := rsa.SignPKCS1v15(rand.Reader, ks.PrivKey, crypto.SHA256, hashed[:])
	if err != nil {
		return nil, fmt.Errorf("signing: %w", err)
	}

	// Build the v2 signature block
	v2Block := buildV2Block(signedData, sig, ks.PrivKey, certDER)

	// Wrap in APK signing block
	signingBlock := buildAPKSigningBlock(v2Block)

	// Insert signing block between section 1 and central directory
	newCDOffset := uint64(cdOffset) + uint64(len(signingBlock))
	patchedEOCD := patchEOCDOffset(apkData[eocdOff:], uint32(newCDOffset))

	var result bytes.Buffer
	result.Write(section1)
	result.Write(signingBlock)
	result.Write(section3)
	result.Write(patchedEOCD)
	return result.Bytes(), nil
}

// computeAPKDigest computes the top-level APK digest over three sections.
// Spec: each section is split into 1MB chunks. Each chunk digest:
//
//	SHA-256(0xa5 || uint32LE(chunkLen) || chunkData)
//
// Top-level digest:
//
//	SHA-256(0x5a || uint32LE(numChunks) || chunk1digest || ... || chunkNdigest)
func computeAPKDigest(section1, section3, section4 []byte) ([]byte, error) {
	sections := [][]byte{section1, section3, section4}
	var allChunkDigests []byte
	totalChunks := 0

	for _, sec := range sections {
		chunks := splitIntoChunks(sec, 1<<20) // 1MB chunks
		for _, chunk := range chunks {
			d := chunkDigest(chunk)
			allChunkDigests = append(allChunkDigests, d...)
			totalChunks++
		}
	}

	// Top-level: 0x5a || uint32LE(numChunks) || all chunk digests
	var topInput bytes.Buffer
	topInput.WriteByte(0x5a)
	var tmp [4]byte
	binary.LittleEndian.PutUint32(tmp[:], uint32(totalChunks))
	topInput.Write(tmp[:])
	topInput.Write(allChunkDigests)

	h := sha256.Sum256(topInput.Bytes())
	return h[:], nil
}

func splitIntoChunks(data []byte, chunkSize int) [][]byte {
	if len(data) == 0 {
		// Empty section → zero chunks. The top-level digest still counts the 0 chunks.
		return nil
	}
	var chunks [][]byte
	for len(data) > 0 {
		n := chunkSize
		if n > len(data) {
			n = len(data)
		}
		chunks = append(chunks, data[:n])
		data = data[n:]
	}
	return chunks
}

// chunkDigest computes SHA-256(0xa5 || uint32LE(len) || data).
func chunkDigest(chunk []byte) []byte {
	var buf bytes.Buffer
	buf.WriteByte(0xa5)
	var tmp [4]byte
	binary.LittleEndian.PutUint32(tmp[:], uint32(len(chunk)))
	buf.Write(tmp[:])
	buf.Write(chunk)
	h := sha256.Sum256(buf.Bytes())
	return h[:]
}

// buildSignedData constructs the raw signed-data bytes for one signer.
//
// Format (no outer length prefix — the caller stores it with lpBytes in the v2 block):
//
//	[4: len(digests-seq)] [digests-seq bytes]
//	[4: len(certs-seq)]   [certs-seq bytes]
//	[4: len(attrs-seq)]   [attrs-seq bytes]   (empty attrs = [4: 0])
//
// digests-seq element:  [4: 4+4+32=40] [sigAlgID(4)] [4: 32] [digest(32)]
// certs-seq element:    [4: certLen]   [cert DER bytes]
//
// Reference: AOSP tools/apksig/src/main/java/com/android/apksig/internal/apk/v2/V2SchemeVerifier.java
func buildSignedData(digest, certDER []byte) []byte {
	// One digest entry: [4: 4+4+32=40][sigAlgID][4: 32][digest]
	digestEntry := lpBytes(concatBytes(appendUint32LE(nil, sigAlgRSAPKCS1SHA256), lpBytes(digest)))
	// digestsSeq: the sequence (single entry, no outer length prefix)
	digestsSeq := digestEntry

	// One cert entry: [4: certLen][cert bytes]
	certsSeq := lpBytes(certDER)

	// Additional attributes: empty sequence (0 bytes of content)
	attrsSeq := []byte{}

	// signed-data = [4: len(digestsSeq)][digestsSeq] || [4: len(certsSeq)][certsSeq] || [4: 0]
	// NO outer length prefix — the v2 block signer stores it with its own lpBytes wrapper.
	return concatBytes(lpBytes(digestsSeq), lpBytes(certsSeq), lpBytes(attrsSeq))
}

// buildV2Block constructs the APK signature scheme v2 block for one signer.
//
// signedData must be the raw signed-data bytes from buildSignedData (no outer lpBytes).
// The signer is: [4: len(signedData)][signedData] || [4: len(sigSeq)][sigSeq] || [4: len(pubKey)][pubKey]
//
// Reference: AOSP tools/apksig/src/main/java/com/android/apksig/internal/apk/v2/V2SchemeSigner.java
func buildV2Block(signedData, sig []byte, privKey *rsa.PrivateKey, certDER []byte) []byte {
	// One signature entry: [4: 4+4+sigLen=pair_len][sigAlgID][4: sigLen][sig]
	sigEntry := lpBytes(concatBytes(appendUint32LE(nil, sigAlgRSAPKCS1SHA256), lpBytes(sig)))
	// signaturesSeq: single entry (no outer prefix)
	signaturesSeq := sigEntry

	// SubjectPublicKeyInfo (raw DER, no extra prefix here; signer wraps it)
	pubKeyDER, _ := x509.MarshalPKIXPublicKey(&privKey.PublicKey)

	// Signer content:
	//   [4: signedDataLen][signedData]          (signedData stored with lpBytes in block)
	//   [4: sigSeqLen][signaturesSeq]
	//   [4: pubKeyLen][pubKeyDER]
	signerContent := concatBytes(
		lpBytes(signedData),    // signedData stored with length prefix
		lpBytes(signaturesSeq), // signatures stored with length prefix
		lpBytes(pubKeyDER),     // public key stored with length prefix
	)
	signer := lpBytes(signerContent)

	// signers = [4: signerLen][signer]  (length-prefixed sequence of one signer)
	return lpBytes(signer)
}

// buildAPKSigningBlock wraps the v2 block in the APK Signing Block container.
// Format:
//
//	size-of-block (uint64) — size of everything except this field and magic
//	ID-value pairs:
//	  pair-len (uint64)
//	  pair-id (uint32)
//	  pair-value (bytes)
//	size-of-block (uint64) — same value as above
//	magic (16 bytes)
func buildAPKSigningBlock(v2Block []byte) []byte {
	// One ID-value pair: (8 len + 4 id + v2Block)
	pairLen := uint64(4 + len(v2Block)) // id(4) + value
	pairBytes := make([]byte, 8+4+len(v2Block))
	binary.LittleEndian.PutUint64(pairBytes[0:], pairLen)
	binary.LittleEndian.PutUint32(pairBytes[8:], v2BlockID)
	copy(pairBytes[12:], v2Block)

	// size-of-block = len(pairs) + 8 (second size field) + 16 (magic)
	// but actually: size-of-block = total block size - 8 (first size field)
	// The two size fields + magic = 8 + 8 + 16 = 32 bytes
	// Total block = 8 + len(pairs) + 8 + 16 = 32 + len(pairs)
	blockSize := uint64(len(pairBytes)) + 8 + 16 // pairs + second-size + magic

	var buf bytes.Buffer
	var tmp8 [8]byte
	binary.LittleEndian.PutUint64(tmp8[:], blockSize)
	buf.Write(tmp8[:]) // first size field
	buf.Write(pairBytes)
	binary.LittleEndian.PutUint64(tmp8[:], blockSize)
	buf.Write(tmp8[:]) // second size field
	buf.WriteString(apkSigningBlockMagic)
	return buf.Bytes()
}

// findEOCDOffset searches backwards for the End of Central Directory record.
func findEOCDOffset(data []byte) (int, error) {
	const sig = uint32(0x06054b50)
	for i := len(data) - 22; i >= 0; i-- {
		if binary.LittleEndian.Uint32(data[i:]) == sig {
			return i, nil
		}
	}
	return 0, fmt.Errorf("EOCD signature not found")
}

// parseCDFromEOCD extracts the Central Directory offset and size from the EOCD.
// EOCD layout (after 4-byte signature):
// diskNum(2) cdStartDisk(2) cdEntriesOnDisk(2) cdEntries(2) cdSize(4) cdOffset(4) commentLen(2)
func parseCDFromEOCD(data []byte, eocdOff int) (cdOffset, cdSize int, err error) {
	eocd := data[eocdOff:]
	if len(eocd) < 22 {
		return 0, 0, fmt.Errorf("EOCD too short")
	}
	cdSize = int(binary.LittleEndian.Uint32(eocd[12:]))
	cdOffset = int(binary.LittleEndian.Uint32(eocd[16:]))
	return cdOffset, cdSize, nil
}

// patchEOCDOffset returns a copy of the EOCD with the CD offset field set to newOffset.
// Per the APK v2 spec, when computing the section 4 digest, the CD offset must be
// set to the value it will have in the final signed APK (i.e. pointing past the signing block).
func patchEOCDOffset(eocd []byte, newOffset uint32) []byte {
	patched := make([]byte, len(eocd))
	copy(patched, eocd)
	binary.LittleEndian.PutUint32(patched[16:], newOffset)
	return patched
}

// ---- helpers ----

func lpBytes(data []byte) []byte {
	buf := make([]byte, 4+len(data))
	binary.LittleEndian.PutUint32(buf[0:], uint32(len(data)))
	copy(buf[4:], data)
	return buf
}

func appendUint32LE(buf []byte, v uint32) []byte {
	tmp := make([]byte, 4)
	binary.LittleEndian.PutUint32(tmp, v)
	return append(buf, tmp...)
}

func concatBytes(parts ...[]byte) []byte {
	var out []byte
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}
