// Copyright 2023 Rivian Automotive, Inc.
// Licensed under the Apache License, Version 2.0 (the “License”);
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an “AS IS” BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package filestore

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/rivian/delta-go/storage"
)

// FileObjectStore provides local file storage
type FileObjectStore struct {
	BaseURI *storage.Path
}

// Compile time check that FileObjectStore implements storage.ObjectStore
var _ storage.ObjectStore = (*FileObjectStore)(nil)

func New(baseURI *storage.Path) *FileObjectStore {
	fs := new(FileObjectStore)
	fs.BaseURI = baseURI
	return fs
}

func (s *FileObjectStore) Put(location *storage.Path, bytes []byte) error {
	writePath := filepath.Join(s.BaseURI.Raw, location.Raw)
	err := os.MkdirAll(filepath.Dir(writePath), 0700)
	if err != nil {
		return err
	}
	err = os.WriteFile(writePath, bytes, 0700)
	return err
}

func (s *FileObjectStore) RenameIfNotExists(from *storage.Path, to *storage.Path) error {

	// return ErrorVersionAlreadyExists if the destination file exists
	_, err := s.Head(to)
	if !errors.Is(err, storage.ErrorObjectDoesNotExist) {
		return fmt.Errorf("error %w: Object at location %s already exists", storage.ErrorVersionAlreadyExists, to.Raw)
	}
	// rename source to destination
	err = s.Rename(from, to)
	if err != nil {
		return err
	}
	return nil

}

func (s *FileObjectStore) Get(location *storage.Path) ([]byte, error) {
	filePath := filepath.Join(s.BaseURI.Raw, location.Raw)
	data, err := os.ReadFile(filePath)
	return data, err
}

func (s *FileObjectStore) Head(location *storage.Path) (storage.ObjectMeta, error) {
	filePath := filepath.Join(s.BaseURI.Raw, location.Raw)
	var meta storage.ObjectMeta
	info, err := os.Stat(filePath)
	if os.IsNotExist(err) {
		return meta, errors.Join(storage.ErrorObjectDoesNotExist, err)
	}
	meta.Size = info.Size()
	meta.Location = storage.Path{Raw: filePath}
	meta.LastModified = info.ModTime()

	if info.IsDir() {
		return meta, storage.ErrorObjectIsDir
	}

	return meta, nil
}

func (s *FileObjectStore) Rename(from *storage.Path, to *storage.Path) error {
	// rename source to destination
	f := s.BaseURI.Join(from)
	t := s.BaseURI.Join(to)
	err := os.Rename(f.Raw, t.Raw)
	if err != nil {
		return errors.Join(storage.ErrorObjectDoesNotExist, err)
	}
	return err
}

func (s *FileObjectStore) Delete(location *storage.Path) error {
	filePath := filepath.Join(s.BaseURI.Raw, location.Raw)
	err := os.Remove(filePath)
	if err != nil {
		return errors.Join(storage.ErrorDeleteObject, err)
	}
	return nil
}

// / Convert an fs.FileInfo to a storage.ObjectMeta
func objectMetaFromFileInfo(info fs.FileInfo, name string, isDir bool, parentDir string, trimPrefix string) (*storage.ObjectMeta, error) {
	meta := new(storage.ObjectMeta)
	meta.LastModified = info.ModTime()
	// Combine the parent directory and the name, and then trim off the prefix
	location := strings.TrimPrefix(path.Join(parentDir, name), trimPrefix)
	if isDir {
		meta.Size = 0
		// For consistency with S3, directories end with a /
		if !os.IsPathSeparator(location[len(location)-1]) {
			location += string(filepath.Separator)
		}
	} else {
		meta.Size = info.Size()
	}
	meta.Location = *storage.NewPath(location)
	return meta, nil
}

// / Convert an fs.DirEntry to a storage.ObjectMeta
func objectMetaFromDirEntry(dirEntry fs.DirEntry, parentDir string, trimPrefix string) (*storage.ObjectMeta, error) {
	info, err := dirEntry.Info()
	if err != nil {
		return nil, err
	}
	return objectMetaFromFileInfo(info, dirEntry.Name(), dirEntry.IsDir(), parentDir, trimPrefix)
}

// / List all files in the directory recursively, where the file must start with prefix if it is not empty
// / For consistency with S3, directory names are included
// / The baseURI will be trimmed from the beginning of each file path
func listFilesInDirRecursively(baseURI string, dir string, prefix string) ([]storage.ObjectMeta, error) {
	results, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	out := make([]storage.ObjectMeta, 0, len(results)+1)

	for _, r := range results {
		if prefix == "" || strings.HasPrefix(r.Name(), prefix) {
			meta, err := objectMetaFromDirEntry(r, dir, baseURI)
			if err != nil {
				return nil, err
			}
			out = append(out, *meta)

			if r.IsDir() {
				subdirResults, err := listFilesInDirRecursively(baseURI, path.Join(dir, r.Name()), "")
				if err != nil {
					return nil, err
				}
				out = append(out, subdirResults...)
			}
		}
	}
	return out, nil
}

func (s *FileObjectStore) List(prefix *storage.Path) ([]storage.ObjectMeta, error) {
	dir, filePrefix := filepath.Split(prefix.Raw)

	fullDir := filepath.Join(s.BaseURI.Raw, dir)

	// If filePrefix was "", make sure fullDir includes a trailing separator.
	// Otherwise we will return results in the parent directory that start with the same
	// string as our store folder name.
	if filePrefix == "" && !os.IsPathSeparator(fullDir[len(fullDir)-1]) {
		fullDir += string(filepath.Separator)
	}

	// baseURI will be trimmed from the beginning of the results returned.
	// It must have a trailing separator.
	baseURI := s.BaseURI.Raw
	if !os.IsPathSeparator(baseURI[len(baseURI)-1]) {
		baseURI += string(filepath.Separator)
	}

	files, err := listFilesInDirRecursively(baseURI, fullDir, filePrefix)
	if err != nil {
		return nil, errors.Join(storage.ErrorListObjects, err)
	}

	// If the prefix passed in was a directory, add the root directory explicitly
	if dir != "" && filePrefix == "" {
		info, err := os.Stat(filepath.Join(s.BaseURI.Raw, dir))
		// If we get an error the directory doesn't exist, that's okay
		if err != nil && !os.IsNotExist(err) {
			return nil, errors.Join(storage.ErrorListObjects, err)
		}
		if err == nil {
			meta, err := objectMetaFromFileInfo(info, dir, true, "", baseURI)
			if err != nil {
				return nil, errors.Join(storage.ErrorListObjects, err)
			}
			files = append(files, *meta)
		}
	}
	return files, nil
}
