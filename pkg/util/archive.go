package util

import (
	"archive/tar"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/format/index"

	"github.com/werf/logboek"
)

func CreateArchiveBasedOnAnotherOne(ctx context.Context, sourceArchivePath, destinationArchivePath string, pathsToExclude []string, f func(tw *tar.Writer) error) error {
	return CreateArchive(destinationArchivePath, func(tw *tar.Writer) error {
		source, err := os.Open(sourceArchivePath)
		if err != nil {
			return fmt.Errorf("unable to open %q: %s", sourceArchivePath, err)
		}
		defer source.Close()

		tr := tar.NewReader(source)

	ArchiveCopying:
		for {
			hdr, err := tr.Next()
			if err == io.EOF {
				break
			} else if err != nil {
				return fmt.Errorf("unable to read archive %q: %s", sourceArchivePath, err)
			}

			for _, pathToExclude := range pathsToExclude {
				if hdr.Name == filepath.ToSlash(pathToExclude) {
					if debugArchiveUtil() {
						logboek.Context(ctx).Debug().LogF("Source archive file was excluded: %q\n", hdr.Name)
					}

					continue ArchiveCopying
				}
			}

			if err := tw.WriteHeader(hdr); err != nil {
				return fmt.Errorf("unable to write header %q from %q archive to %q: %s", hdr.Name, sourceArchivePath, destinationArchivePath, err)
			}

			if _, err := io.Copy(tw, tr); err != nil {
				return fmt.Errorf("unable to copy file %q from %q archive to %q: %s", hdr.Name, sourceArchivePath, destinationArchivePath, err)
			}

			if debugArchiveUtil() {
				logboek.Context(ctx).Debug().LogF("Source archive file was added: %q\n", hdr.Name)
			}
		}

		return f(tw)
	})
}

func CreateArchive(archivePath string, f func(tw *tar.Writer) error) error {
	if err := os.MkdirAll(filepath.Dir(archivePath), 0777); err != nil {
		return fmt.Errorf("unable to create dir %q: %s", filepath.Dir(archivePath), err)
	}

	file, err := os.Create(archivePath)
	if err != nil {
		return fmt.Errorf("unable to create %q: %s", archivePath, err)
	}
	defer file.Close()

	tw := tar.NewWriter(file)
	defer tw.Close()

	return f(tw)
}

func CopyFileIntoTar(tw *tar.Writer, tarEntryName string, filePath string) error {
	stat, err := os.Lstat(filePath)
	if err != nil {
		return fmt.Errorf("unable to stat file %q: %s", filePath, err)
	}

	if stat.Mode().IsDir() {
		return fmt.Errorf("directory %s cannot be added to tar archive", filePath)
	}

	header := &tar.Header{
		Name:       tarEntryName,
		Mode:       int64(stat.Mode()),
		Size:       stat.Size(),
		ModTime:    stat.ModTime(),
		AccessTime: stat.ModTime(),
		ChangeTime: stat.ModTime(),
	}

	if stat.Mode()&os.ModeSymlink != 0 {
		linkname, err := os.Readlink(filePath)
		if err != nil {
			return fmt.Errorf("unable to read link %q: %s", filePath, err)
		}

		header.Linkname = linkname
		header.Typeflag = tar.TypeSymlink
	}

	if err := tw.WriteHeader(header); err != nil {
		return fmt.Errorf("unable to write tar header for file %s: %s", tarEntryName, err)
	}

	if stat.Mode().IsRegular() {
		f, err := os.Open(filePath)
		if err != nil {
			return fmt.Errorf("unable to open file %q: %s", filePath, err)
		}
		defer f.Close()

		data, err := ioutil.ReadAll(f)
		if err != nil {
			return nil
		}

		if _, err := tw.Write(data); err != nil {
			return fmt.Errorf("unable to write data to tar archive from file %q: %s", filePath, err)
		}
	}

	return nil
}

func CopyGitIndexEntryIntoTar(tw *tar.Writer, tarEntryName string, entry *index.Entry, obj plumbing.EncodedObject) error {
	r, err := obj.Reader()
	if err != nil {
		return err
	}

	header := &tar.Header{
		Name:       tarEntryName,
		Mode:       int64(entry.Mode),
		Size:       int64(entry.Size),
		ModTime:    entry.ModifiedAt,
		AccessTime: entry.ModifiedAt,
		ChangeTime: entry.ModifiedAt,
	}

	if entry.Mode == filemode.Symlink {
		data, err := ioutil.ReadAll(r)
		if err != nil {
			return err
		}

		linkname := string(data)
		header.Linkname = linkname
		header.Typeflag = tar.TypeSymlink
	}

	if err := tw.WriteHeader(header); err != nil {
		return fmt.Errorf("unable to write tar header for git index entry %s: %s", tarEntryName, err)
	}

	if entry.Mode.IsFile() {
		if _, err := io.Copy(tw, r); err != nil {
			return fmt.Errorf("unable to write data to tar for git index entry %q: %s", tarEntryName, err)
		}
	}

	return nil
}

func debugArchiveUtil() bool {
	return os.Getenv("WERF_DEBUG_ARCHIVE_UTIL") == "1"
}
