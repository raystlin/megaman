package mega

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"strings"

	errors "golang.org/x/xerrors"
)

// blockDecrypt decrypts using the block cipher blk in ECB mode.
func blockDecrypt(blk cipher.Block, dst, src []byte) error {

	if len(src) > len(dst) || len(src)%blk.BlockSize() != 0 {
		return errors.New("Block decryption failed")
	}

	l := len(src) - blk.BlockSize()

	for i := 0; i <= l; i += blk.BlockSize() {
		blk.Decrypt(dst[i:], src[i:])
	}

	return nil
}

// decryptAttr decrypts what it expects to be an FileAttr structure
func decryptAttr(key []byte, data string) (attr FileAttr, err error) {
	err = errors.New("Bad Attr")
	block, err := aes.NewCipher(key)
	if err != nil {
		return attr, err
	}
	iv, err := a32_to_bytes([]uint32{0, 0, 0, 0})
	if err != nil {
		return attr, err
	}
	mode := cipher.NewCBCDecrypter(block, iv)
	buf := make([]byte, len(data))
	ddata, err := base64urldecode(data)
	if err != nil {
		return attr, err
	}
	mode.CryptBlocks(buf, ddata)

	if string(buf[:4]) == "MEGA" {
		str := strings.TrimRight(string(buf[4:]), "\x00")
		trimmed := attrMatch.FindString(str)
		if trimmed != "" {
			str = trimmed
		}
		err = json.Unmarshal([]byte(str), &attr)
	}
	return attr, err
}

// bytes_to_a32 converts a byte slice to a uint32 slice
// where each uint32 is encoded in big endian order
func bytes_to_a32(b []byte) ([]uint32, error) {
	length := len(b) + 2
	a := make([]uint32, length/4)
	buf := bytes.NewBuffer(b)
	for i, _ := range a {
		err := binary.Read(buf, binary.BigEndian, &a[i])
		if err != nil {
			return nil, err
		}
	}

	return a, nil
}

// a32_to_bytes converts the uint32 slice a to byte slice where each
// uint32 is decoded in big endian order.
func a32_to_bytes(a []uint32) ([]byte, error) {
	buf := new(bytes.Buffer)
	buf.Grow(len(a) * 3) // To prevent reallocations in Write
	for _, v := range a {
		err := binary.Write(buf, binary.BigEndian, v)
		if err != nil {
			return nil, err
		}
	}

	return buf.Bytes(), nil
}

// convenience function to decode base64 with url encoding and no padding.
func base64urldecode(data string) ([]byte, error) {
	return base64.URLEncoding.WithPadding(base64.NoPadding).DecodeString(data)
}
