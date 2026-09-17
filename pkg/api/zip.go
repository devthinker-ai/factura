package api

import (
	"archive/zip"
	"io"
)

func writeZip(w io.Writer, files map[string][]byte) error {
	zw := zip.NewWriter(w)
	for name, data := range files {
		fw, err := zw.Create(name)
		if err != nil {
			_ = zw.Close()
			return err
		}
		if _, err := fw.Write(data); err != nil {
			_ = zw.Close()
			return err
		}
	}
	return zw.Close()
}
