package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/urfave/cli/v2"
)

func CmdExport(c *cli.Context) error {
	names := c.StringSlice("name")
	dest := c.String("dest")

	store := MustInitStore(c)

	for _, name := range names {
		resource, err := store.FetchResource(name)
		if err != nil {
			panic(err)
		}

		err = writeFile(resource.PrivateKey, dest, name, "key")
		if err != nil {
			panic(err)
		}
		err = writeFile(resource.Certificate, dest, name, "crt")
		if err != nil {
			panic(err)
		}
	}
	fmt.Println("done")
	return nil
}

func writeFile(content []byte, base, name, ext string) error {
	fileName := filepath.Join(base, fmt.Sprintf("%s.%s", name, ext))
	f, err := os.Create(fileName)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(content)
	return err
}
