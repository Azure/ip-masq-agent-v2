/*
Copyright 2017 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package fakefs

import (
	"errors"
	"os"
	"strings"
	"time"
)

type FileSystem interface {
	Stat(name string) (os.FileInfo, error)
	ReadFile(name string) ([]byte, error)
	ReadDir(name string) ([]os.DirEntry, error)
}

// DefaultFS implements FileSystem using the local disk
type DefaultFS struct{}

func (DefaultFS) Stat(name string) (os.FileInfo, error) {
	return os.Stat(name)
}
func (DefaultFS) ReadFile(name string) ([]byte, error) {
	return os.ReadFile(name)
}
func (DefaultFS) ReadDir(name string) ([]os.DirEntry, error) {
	return os.ReadDir(name)
}

type File struct {
	Name    string
	Content string
}

// StringFS holds a slice of files
type StringFS struct {
	Files []File
}

func (fs StringFS) Stat(name string) (os.FileInfo, error) {
	f := NewFileInfo()

	for _, file := range fs.Files {
		if strings.EqualFold(file.Name, name) {
			f.name = name
			f.size = int64(len([]byte(file.Content)))
			break
		}
	}

	return f, nil
}
func (fs StringFS) ReadFile(name string) ([]byte, error) {
	for _, file := range fs.Files {
		if strings.EqualFold("/etc/config/"+file.Name, name) {
			return []byte(file.Content), nil
		}
	}

	return nil, &os.PathError{
		Op:   "open",
		Path: name,
		Err:  errors.New("errno 2"), // errno 2 is ENOENT, since the file shouldn't exist
	}
}
func (fs StringFS) ReadDir(name string) ([]os.DirEntry, error) {
	var entries []os.DirEntry

	for _, file := range fs.Files {
		f := NewFileInfo()
		f.name = file.Name
		f.size = int64(len([]byte(file.Content)))
		entries = append(entries, f)
	}

	return entries, nil
}

// NotExistFS will always return os.ErrNotExist type errors from calls to Stat
type NotExistFS struct{}

func (NotExistFS) Stat(name string) (os.FileInfo, error) {
	return nil, os.ErrNotExist
}
func (NotExistFS) ReadFile(name string) ([]byte, error) {
	return []byte{},
		&os.PathError{
			Op:   "open",
			Path: name,
			Err:  errors.New("errno 2"), // errno 2 is ENOENT, since the file shouldn't exist
		}
}
func (NotExistFS) ReadDir(name string) ([]os.DirEntry, error) {
	return []os.DirEntry{},
		&os.PathError{
			Op:   "open",
			Path: name,
			Err:  errors.New("errno 2"), // errno 2 is ENOENT, since the dir shouldn't exist
		}
}

// FileInfo implements the os.FileInfo and os.DirEntry interface
type FileInfo struct {
	name    string
	size    int64
	mode    os.FileMode
	modTime time.Time
	isDir   bool
	sys     interface{}
}

func (f *FileInfo) Name() string               { return f.name }
func (f *FileInfo) Size() int64                { return f.size }
func (f *FileInfo) Mode() os.FileMode          { return f.mode }
func (f *FileInfo) ModTime() time.Time         { return f.modTime }
func (f *FileInfo) IsDir() bool                { return f.isDir }
func (f *FileInfo) Sys() interface{}           { return f.sys }
func (f *FileInfo) Type() os.FileMode          { return f.mode }
func (f *FileInfo) Info() (os.FileInfo, error) { return f, nil }

func NewFileInfo() *FileInfo {
	return &FileInfo{
		name:    "",
		size:    0,
		mode:    os.FileMode(0777),
		modTime: time.Time{}, // just use zero time
		isDir:   false,
		sys:     nil,
	}
}
