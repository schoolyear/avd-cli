package lib

import (
	"fmt"
	"github.com/friendsofgo/errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

func EnsureEmptyDirectory(path string, overwriteOnCollision bool) error {
	if overwriteOnCollision {
		if err := os.RemoveAll(path); err != nil {
			return errors.Wrapf(err, "failed to delete path: %s", path)
		}
	} else {
		fileInfo, err := os.Stat(path)
		if err != nil {
			if !os.IsNotExist(err) {
				return errors.Wrapf(err, "failed to check path for existing folder/file: %s", path)
			}
		} else {
			if fileInfo.IsDir() {
				return fmt.Errorf("directory already exists: %s", path)
			} else {
				return fmt.Errorf("directory is already a file: %s", path)
			}
		}
	}

	return os.MkdirAll(path, os.ModePerm)
}

func CopyFile(sourceFs fs.FS, sourcePath, targetPath string) error {
	sourceFile, err := sourceFs.Open(sourcePath)
	if err != nil {
		return errors.Wrapf(err, "failed to open source file for copying %s", targetPath)
	}
	defer sourceFile.Close()

	targetFile, err := os.Create(targetPath)
	if err != nil {
		return errors.Wrapf(err, "failed to create new file %s", targetPath)
	}
	defer targetFile.Close()

	if _, err := io.Copy(targetFile, sourceFile); err != nil {
		return errors.Wrapf(err, "failed to write template file %s", targetPath)
	}

	return nil
}

func CalcDirSizeRecursively(fsys fs.FS) (int64, error) {
	var size int64
	err := fs.WalkDir(fsys, ".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			info, err := fs.Stat(fsys, path)
			if err != nil {
				return errors.Wrapf(err, "failed to get file info on %s", path)
			}
			size += info.Size()
		}
		return nil
	})
	return size, err
}

type EmptyFs struct{}

func (e EmptyFs) ReadDir(_ string) ([]fs.DirEntry, error) {
	return []fs.DirEntry{}, nil
}

func (e EmptyFs) Open(_ string) (fs.File, error) {
	return nil, os.ErrNotExist
}

func CopyDirectory(sourceFs fs.FS, sourceBasePath string, targetBasePath string) error {
	defer func() {
		fmt.Println("")
	}()
	return fs.WalkDir(sourceFs, sourceBasePath, func(sourcePath string, d fs.DirEntry, err error) error {
		if err != nil {
			return errors.Wrapf(err, "failed to walk path %s", sourcePath)
		}

		rel, err := filepath.Rel(sourceBasePath, sourcePath)
		if err != nil {
			return errors.Wrapf(err, "failed to get rel to base path for %s", sourcePath)
		}
		targetPath := filepath.Join(targetBasePath, rel)
		targetPathBase := filepath.Base(targetBasePath)

		if d.IsDir() {
			fmt.Printf("[DIR ]: %s", filepath.Join(targetPathBase, rel))
			var mkDirFn = os.Mkdir
			if rel == "." {
				mkDirFn = os.MkdirAll
			}
			if err := mkDirFn(targetPath, os.ModePerm); err != nil {
				return errors.Wrapf(err, "failed to create directory: %s", targetPath)
			}
			fmt.Printf(" - OK\n")
		} else {
			fmt.Printf("[FILE]: %s", filepath.Join(targetPathBase, rel))
			if err := CopyFile(sourceFs, sourcePath, targetPath); err != nil {
				return errors.Wrap(err, "failed to copy file")
			}
			fmt.Printf(" - OK\n")
		}

		return nil
	})
}
