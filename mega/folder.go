package mega

import (
	"crypto/aes"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	errors "golang.org/x/xerrors"
)

// Regexp to extract url information
var (
	folderURLRE = regexp.MustCompile(`https://mega.nz/folder/([A-Za-z0-9-_]*)#([A-Za-z0-9-_]*)`)
	attrMatch   = regexp.MustCompile(`{".*"}`)
)

// Filesystem node types
const (
	FileType   = 0
	FolderType = 1
	RootType   = 2
	InboxType  = 3
	TrashType  = 4
)

// A folder list request
type folderReq struct {
	A  string `json:"a"`
	C  int    `json:"c"`
	CA int    `json:"ca"`
	R  int    `json:"r"`
}

// The response
type folderResp struct {
	F   []FSNode `json:"f"`
	Noc int      `json:"noc"`
	Sn  string   `json:"sn"`
}

// A folder response node that represents
// a file/folder/trash/inbox/root
type FSNode struct {
	Hash   string `json:"h"`
	Parent string `json:"p"`
	User   string `json:"u"`
	T      int    `json:"t"`
	Attr   string `json:"a"`
	Key    string `json:"k"`
	Ts     int64  `json:"ts"`
	Sz     int64  `json:"s"`
	Fa     string `json:"fa"`
	meta   NodeMeta
}

// Until the node is decoded is pretty useless
func (n FSNode) IsDecoded() bool {
	return len(n.meta.key) > 0
}

// Returns the name of the node, if it has been decoded
func (n FSNode) Name() string {
	if !n.IsDecoded() {
		return ""
	}
	return n.Attr
}

func (n FSNode) IsFile() bool {
	return n.T == FileType
}

func (n FSNode) IsDir() bool {
	return n.T == FolderType
}

func (n FSNode) IsRoot() bool {
	return n.T == RootType
}

func (n FSNode) IsInbox() bool {
	return n.T == InboxType
}

func (n FSNode) IsTrash() bool {
	return n.T == TrashType
}

func (n FSNode) IsUnknown() bool {
	return n.T < 0 || n.T > 4
}

// Crypto metadata for the node
type NodeMeta struct {
	key     []byte
	compkey []byte
	iv      []byte
	mac     []byte
	context string
}

// A filesystem tree
type FileSystem struct {
	Roots []*TreeNode
}

// Filesystem tree node
type TreeNode struct {
	Node     *FSNode
	Children []*TreeNode
}

// Get handle and key information from folders
func parseFolderURL(url string) (handle, key string, err error) {
	split := folderURLRE.FindStringSubmatch(url)
	if len(split) == 0 {
		err = errInvalidURL
		return
	}

	handle, key = split[1], split[2]
	if len(handle) != 8 || len(key) != 22 {
		err = errInvalidURL
		return
	}

	return
}

// Calculates the querystring values for folder requests
func makeValues(handle string) url.Values {
	values := make(url.Values)
	values.Add("n", handle)
	values.Add("ec", "")
	values.Add("v", "2")
	values.Add("domain", "meganz")

	return values
}

// List the contents of a folder URL
func List(folderURL string) (*FileSystem, error) {
	handle, key, err := parseFolderURL(folderURL)
	if err != nil {
		return nil, err
	}

	// request file information
	resp := folderResp{}
	err = apiReq(&folderReq{
		A:  "f",
		C:  1,
		CA: 1,
		R:  1,
	}, &resp, makeValues(handle))
	if err != nil {
		return nil, err
	}

	searchTree := make(map[string]*TreeNode)
	for i := range resp.F {
		node, ok := searchTree[resp.F[i].Hash]
		if !ok {
			item, err := decodeNode(key, &resp.F[i])
			if err != nil {
				return nil, errors.Errorf("Error decoding node with key %s: %w", key, err)
			}
			item.meta.context = handle

			node = &TreeNode{
				Node:     item,
				Children: make([]*TreeNode, 0),
			}
		} else {
			node.Node = &resp.F[i]
		}
		searchTree[resp.F[i].Hash] = node

		parent, ok := searchTree[resp.F[i].Parent]
		if !ok {
			parent = &TreeNode{
				Children: []*TreeNode{node},
			}
		} else {
			parent.Children = append(parent.Children, node)
		}
		searchTree[resp.F[i].Parent] = parent
	}

	roots := make([]*TreeNode, 0)
	for _, v := range searchTree {
		if v.Node == nil {
			roots = append(roots, v.Children...)
		}
	}

	return &FileSystem{
		Roots: roots,
	}, nil
}

// Gets the cryto metadata and the filename for a node
func decodeNode(k string, node *FSNode) (*FSNode, error) {
	if !node.IsDir() && !node.IsFile() {
		return nil, errors.Errorf("Invalid type %d", node.T)
	}

	args := strings.Split(node.Key, ":")
	if len(args) < 2 {
		return nil, errors.Errorf("not enough : in item.Key: %q", node.Key)
	}

	itemKey := args[1]
	b, err := base64urldecode(k)
	if err != nil {
		return nil, err
	}

	block, err := aes.NewCipher(b)
	if err != nil {
		return nil, err
	}
	buf, err := base64urldecode(itemKey)
	if err != nil {
		return nil, err
	}
	err = blockDecrypt(block, buf, buf)
	if err != nil {
		return nil, err
	}
	compkey, err := bytes_to_a32(buf)
	if err != nil {
		return nil, err
	}

	var key []uint32
	if node.IsFile() {
		if len(compkey) < 8 {
			return nil, nil
		}
		key = []uint32{compkey[0] ^ compkey[4], compkey[1] ^ compkey[5], compkey[2] ^ compkey[6], compkey[3] ^ compkey[7]}
	} else {
		key = compkey
	}

	attr := FileAttr{}
	bkey, err := a32_to_bytes(key)
	if err != nil {
		// FIXME:
		attr.Name = "BAD ATTRIBUTE"
	} else {
		attr, err = decryptAttr(bkey, node.Attr)
		// FIXME:
		if err != nil {
			attr.Name = "BAD ATTRIBUTE"
		}
	}

	if node.IsFile() {
		node.meta.key, err = a32_to_bytes(key)
		if err != nil {
			return nil, err
		}
		node.meta.iv, err = a32_to_bytes([]uint32{compkey[4], compkey[5], 0, 0})
		if err != nil {
			return nil, err
		}
		node.meta.mac, err = a32_to_bytes([]uint32{compkey[6], compkey[7]})
		if err != nil {
			return nil, err
		}
		node.meta.compkey, err = a32_to_bytes(compkey)
		if err != nil {
			return nil, err
		}
	} else if node.IsDir() {
		node.meta.key, err = a32_to_bytes(key)
		if err != nil {
			return nil, err
		}
		node.meta.compkey, err = a32_to_bytes(compkey)
		if err != nil {
			return nil, err
		}
	} else if node.IsRoot() {
		attr.Name = "Cloud Drive"
	} else if node.IsInbox() {
		attr.Name = "InBox"
	} else if node.IsTrash() {
		attr.Name = "Trash"
	} else {
		attr.Name = fmt.Sprintf("Unknown node type %d", node.T)
	}

	node.Attr = attr.Name
	return node, nil
}
