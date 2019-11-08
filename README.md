# Share your files by FTP 📂

> Implementation of File Transfer Protocol (FTP) based on standard library.


## 🚀 Getting Started

### Install via `go get`
```bash
go get github.com/cxfans/ftplib@v0.1.0
```

### Usage

#### Start a FTP Server
```go
func main() {
	addr := "localhost:2121"
	rootDir := "."
	server, err := ftplib.NewServer(addr, rootDir)
	if err != nil {
		log.Println(err)
	}
	log.Fatal(server.ListenAndServe())
}
```

#### Start a FTP Client
```go
func main() {
	// Login
	c, err := ftplib.Connect("localhost:21", "admin", "admin")
	if err != nil {
		panic(err)
	}
	defer c.Quit()

	// Upload
	data := bytes.NewBufferString("Uploads a file to the remote FTP server.")
	err = c.Stor("file", data)
	if err != nil {
		panic(err)
	}

	// Download
	r, err := c.Retr("file")
	if err != nil {
		panic(err)
	} else {
		buf, err := ioutil.ReadAll(r)
		if err != nil {
			panic(err)
		}
		fp, err := os.Create(file)
		if err != nil {
			panic(err)
		}
		fp.Write(buf)
		r.Close()
	}
}
```

## 🔵 License

This project is licensed under the MIT License - see the [LICENSE](LICENSE.md) file for details.