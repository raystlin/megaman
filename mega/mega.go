package mega

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
)

var (
	APIEndpoint = "https://g.api.mega.co.nz"

	errInvalidURL = fmt.Errorf("invalid url")
	errAPI        = fmt.Errorf("api error")

	seqno = 0
)

// Checks if an URL links to a mega folder
func IsFolderURL(url string) bool {
	return folderURLRE.MatchString(url)
}

// Checks if an URL links to a mega file
func IsFileURL(url string) bool {
	_, _, err := parseURL(url)
	return err == nil
}

// Generic api request.
// Mega API requests are POST with a JSON body with an array of commands,
// The response is an array of results.
// vals is added to the querystring of the URL.
func apiReq(request interface{}, response interface{}, vals url.Values) error {
	// put request into an array
	var commands []interface{}
	commands = append(commands, request)

	// convert to json
	cj, err := json.Marshal(commands)
	if err != nil {
		return err
	}

	if vals == nil {
		vals = make(url.Values)
	}

	vals.Add("id", strconv.Itoa(seqno))

	// make a post request
	url := fmt.Sprintf("%s/cs?%s", APIEndpoint, vals.Encode())
	seqno++

	resp, err := http.Post(url, "application/json", bytes.NewBuffer(cj))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// read response
	bodyBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	// remove "[" and "]"
	bodyBytes = bodyBytes[1 : len(bodyBytes)-1]

	// convert json to struct
	err = json.Unmarshal(bodyBytes, response)
	if err != nil {
		if _, err := strconv.ParseInt(string(bodyBytes), 10, 64); err == nil {
			return errAPI
		}
		return err
	}

	return nil
}
