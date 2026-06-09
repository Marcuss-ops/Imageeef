package bundler

import (
	"archive/zip"
	"io"
	"log"
	"os"
)

// mergeZips merges multiple zip files into a single zip file
func mergeZips(destZip string, srcZips []string) error {
	outFile, err := os.Create(destZip)
	if err != nil {
		return err
	}
	defer outFile.Close()

	zipWriter := zip.NewWriter(outFile)
	defer zipWriter.Close()

	for _, srcZip := range srcZips {
		if _, err := os.Stat(srcZip); os.IsNotExist(err) {
			continue // Skip missing chunks
		}

		inFile, err := zip.OpenReader(srcZip)
		if err != nil {
			log.Printf("⚠️ Failed to open chunk %s: %v", srcZip, err)
			continue
		}

		for _, file := range inFile.File {
			// Skip existing files to avoid conflicts, though chunks shouldn't overlap
			header := file.FileHeader
			writer, err := zipWriter.CreateHeader(&header)
			if err != nil {
				inFile.Close()
				return err
			}

			srcFile, err := file.Open()
			if err != nil {
				inFile.Close()
				return err
			}

			_, err = io.Copy(writer, srcFile)
			srcFile.Close()
			if err != nil {
				inFile.Close()
				return err
			}
		}
		inFile.Close()
	}
	return nil
}
