package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/mattn/go-colorable"
	"github.com/schollz/progressbar/v3"
	log "github.com/siruspen/logrus"
	errors "golang.org/x/xerrors"

	"github.com/raystlin/megaman/mega"
)

var (
	UnknownURLTypeError = errors.New("Unknown URL Type")
)

func main() {

	log.SetFormatter(&log.TextFormatter{ForceColors: true})
	log.SetOutput(colorable.NewColorableStdout())

	if len(os.Args) != 3 {
		fmt.Printf("Invalid call: %s <url|file> <output>", os.Args[0])
		return
	}

	var err error
	//First of all we have to determine if we are deailing with a file or
	//an URL. If the file exists we will treat the file
	if info, err := os.Stat(os.Args[1]); err == nil && !info.IsDir() {
		err = processFile(os.Args[1], os.Args[2])
	} else {
		err = processURL(os.Args[1], os.Args[2])
	}

	if err != nil {
		log.WithError(err).Fatal("Failed")
	} else {
		log.Info("Done")
	}
}

// This function reads the whole file, splits its words and looks
// for mega urls, that are sent to processURL function
func processFile(path, output string) error {
	log.Info("Processing file")
	info, err := ioutil.ReadFile(path)
	if err != nil {
		log.WithError(err).WithFields(log.Fields{
			"path": path,
		}).Error("Could not open file")
	}

	urls := strings.Fields(string(info))
	for i := range urls {
		log.Info(urls[i])
		err = processURL(urls[i], output)
		if err != nil && !errors.Is(err, UnknownURLTypeError) {
			return err
		}
	}

	return nil
}

// Downloads an URL that can be a file or a folder.
// If the url is a file we will simply download it
// to the output folder, and if it is a folder we
// will recreate the whole structure starting from
// output.
func processURL(url, output string) error {
	log.Info("Processing URL")
	if mega.IsFileURL(url) {
		reader, info, err := mega.Download(url)
		if err != nil {
			log.WithError(err).WithFields(log.Fields{
				"url": url,
			}).Error("Could not start download")

			return err
		}

		err = downloadOne(reader, info, output)
		if err != nil {
			log.WithError(err).Error("Could not download file")
			return err
		}
	} else if mega.IsFolderURL(url) {
		os.MkdirAll(output, 0755)
		fs, err := mega.List(url)
		if err != nil {
			log.WithError(err).WithFields(log.Fields{
				"path": url,
			}).Error("Could not list folder")

			return err
		}

		for _, root := range fs.Roots {
			err = processNode(root, output)
			if err != nil {
				log.WithError(err).Error("Error processing roots")
			}
			return err
		}
	} else {
		return UnknownURLTypeError
	}

	return nil
}

// Recursive function to process the directory tree
// we generated from mega. Folders are created, files
// are downloaded
func processNode(n *mega.TreeNode, path string) error {
	if n.Node.IsFile() {
		reader, info, err := mega.DownloadNode(n.Node)
		if err != nil {
			log.WithError(err).WithFields(log.Fields{
				"path": path,
				"file": n.Node.Name(),
			}).Error("Could not download file")
		}

		err = downloadOne(reader, info, path)
		if err != nil {
			log.WithError(err).WithFields(log.Fields{
				"path": path,
				"file": n.Node.Name(),
			}).Fatal("Could not download file")
		}
	} else if n.Node.IsDir() {
		newPath := filepath.Join(path, n.Node.Name())
		os.Mkdir(newPath, 0755)

		for _, r := range n.Children {
			err := processNode(r, newPath)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// Downloads a file and shows a progress bar.
func downloadOne(reader io.ReadCloser, info *mega.Info, path string) error {
	defer reader.Close()

	f, err := os.Create(filepath.Join(path, info.Name))
	if err != nil {
		log.WithError(err).WithFields(log.Fields{
			"file": info.Name,
		}).Error("Could not create file")
		return err
	}
	defer f.Close()

	bar := progressbar.DefaultBytes(
		int64(info.Size),
		"Downloading "+info.Name,
	)

	_, err = io.Copy(io.MultiWriter(f, bar), reader)
	if err != nil {
		log.WithError(err).Fatal("Could not download file")
	}
	return err
}
