package ftp

import (
	"fmt"
	"io"
	"log"
	"os"
	"testing"
)

func TestFunctional(t *testing.T) {
	os.Setenv("DEBUG", "1")

	client := Client{
		Host:     "ftp.ensembl.org",
		Port:     21,
		Username: "anonymous",
		Password: "email@email.com",
	}
	err := client.Connect()
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	err = client.Login()
	if err != nil {
		log.Fatal(err)
	}
	directory := "/pub"
	entries, err := client.List(directory)
	if err != nil {
		log.Fatal(err)
	}
	os.MkdirAll("./test-output", 0700)
	for _, entry := range entries {
		fmt.Printf("%+v\n", entry)
		if entry.Directory == false && entry.Link == false {
			file, err := os.Create("./test-output/" + entry.Name)
			if err != nil {
				log.Fatal(err)
			}
			reader, err := client.Retr(directory + "/" + entry.Name)
			if err != nil {
				log.Fatal(err)
			}
			io.Copy(file, reader)
			file.Close()
			err = reader.Close()
			if err != nil {
				log.Fatal(err)
			}
		}
	}
}
