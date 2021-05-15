package mega

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

// A file request
type handleReq struct {
	A      string `json:"a"`
	G      int    `json:"g"`
	Handle string `json:"p,omitempty"`
	N      string `json:"n,omitempty"`
}

// A file response
type handleResp struct {
	Size       int    `json:"s"`
	Attributes string `json:"at"`
	Msd        int    `json:"msd"`
	URL        string `json:"g"`
}

// The encrypted content of at fields
type FileAttr struct {
	Name string `json:"n"`
}

// Info contains the necessary information to download a file from mega.
type Info struct {
	Name     string
	Size     int
	URL      string
	AesKey   []byte
	AesIV    []byte
	Progress chan int // Track the download progess (total downloaded bytes)
}

// Extracts the key and the handle from a file URL
func parseURL(url string) (handle, key string, err error) {
	split := strings.SplitN(url, "!", 3)
	if len(split) != 3 {
		err = errInvalidURL
		return
	}

	if split[0] != "https://mega.nz/#" {
		err = errInvalidURL
		return
	}

	handle, key = split[1], split[2]
	if len(handle) != 8 || len(key) != 43 {
		err = errInvalidURL
		return
	}

	return
}

// A file URL contains a packed key. Key, IV and MAC must be
// extracted from it
func unpackKey(key string) (aesKey, aesIV, mac []byte, err error) {
	decodedKey, err := base64urldecode(key)
	if err != nil {
		return
	}

	aesKey = []byte{}
	for i := 0; i < 16; i++ {
		aesKey = append(aesKey, decodedKey[i]^decodedKey[i+16])
	}
	aesIV = append(decodedKey[16:24][:], 0, 0, 0, 0, 0, 0, 0, 0)
	mac = decodedKey[24:32][:]

	return
}

// Extracts and unpacks the keys from an URL and uses
// them to get the encrypted filename
func getInfo(url string) (*Info, error) {
	handle, key, err := parseURL(url)
	if err != nil {
		return nil, err
	}

	// request file information
	resp := handleResp{}
	err = apiReq(&handleReq{
		A:      "g",
		G:      1,
		Handle: handle,
	}, &resp, nil)
	if err != nil {
		return nil, err
	}

	attr, err := base64urldecode(resp.Attributes)
	if err != nil {
		return nil, err
	}

	aesKey, aesIV, mac, err := unpackKey(key)
	if err != nil {
		return nil, err
	}
	_ = mac

	// decrypt Attributes
	block, err := aes.NewCipher(aesKey)
	if err != nil {
		return nil, err
	}
	mode := cipher.NewCBCDecrypter(block, []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0})

	dst := make([]byte, len(attr))
	mode.CryptBlocks(dst, attr)

	mega := []byte("MEGA")
	if !bytes.HasPrefix(dst, mega) {
		return nil, errAPI
	}
	dst = bytes.TrimPrefix(dst, mega)
	dst = bytes.Trim(dst, "\x00")

	// convert json Attributes to struct
	n := FileAttr{}
	err = json.Unmarshal(dst, &n)
	if err != nil {
		return nil, err
	}

	// return with the collected information
	return &Info{
		Name:     n.Name,
		Size:     resp.Size,
		URL:      resp.URL,
		AesKey:   aesKey,
		AesIV:    aesIV,
		Progress: make(chan int, 10),
	}, nil
}

// ReadCloser implementation for mega files
type fileReadCloser struct {
	Reader   *cipher.StreamReader
	Response *http.Response
	FileInfo *Info
	Total    int
}

func (f *fileReadCloser) Close() error {
	return f.Response.Body.Close()
}

func (f *fileReadCloser) Read(p []byte) (n int, err error) {
	n, err = f.Reader.Read(p)
	f.Total += n
	select {
	case f.FileInfo.Progress <- f.Total:
	default:
	}
	return
}

// Download file from mega and report file information
func Download(url string) (io.ReadCloser, *Info, error) {
	info, err := getInfo(url)
	if err != nil {
		return nil, nil, err
	}

	return downloadWithInfo(info)
}

// Download node from mega and report file information
func DownloadNode(node *FSNode) (io.ReadCloser, *Info, error) {
	// request file information
	resp := handleResp{}
	err := apiReq(&handleReq{
		A: "g",
		G: 1,
		N: node.Hash,
	}, &resp, makeValues(node.meta.context))
	if err != nil {
		return nil, nil, err
	}

	msg, err := decryptAttr(node.meta.key, resp.Attributes)
	if err != nil {
		return nil, nil, err
	}

	// return with the collected information
	return downloadWithInfo(&Info{
		Name:     msg.Name,
		Size:     resp.Size,
		URL:      resp.URL,
		AesKey:   node.meta.key,
		AesIV:    node.meta.iv,
		Progress: make(chan int, 10),
	})
}

// Common functionality to Download and DownloadNode
func downloadWithInfo(info *Info) (io.ReadCloser, *Info, error) {
	resp, err := http.Get(info.URL)
	if err != nil {
		return nil, nil, err
	}

	block, err := aes.NewCipher(info.AesKey)
	if err != nil {
		return nil, nil, err
	}

	stream := cipher.NewCTR(block, info.AesIV)
	reader := &cipher.StreamReader{S: stream, R: resp.Body}

	return &fileReadCloser{
		Reader:   reader,
		Response: resp,
		FileInfo: info,
	}, info, nil
}
