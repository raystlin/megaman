# Megaman
A cli to download public files and folders from mega.nz
This project is just a mixture of the following projects:
* https://github.com/t3rm1n4l/go-mega
* https://github.com/u3mur4/megadl

## Capabilities
* Download a single public file from mega.nz (as megadl does)
* Download a public folder from mega.nz (as go-mega does with private folders). The full tree is copied to the output folder and all the files are downloaded.
* Look for mega.nz urls inside a text and download them.

## Compilation
```shell
go mod tidy
go build megaman.go
```

## Execution
```shell
megaman <file|url> output
```