package goman

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Request struct {
	Name    string            `json:"name" yaml:"name"`
	Method  string            `json:"method" yaml:"method"`
	URL     string            `json:"url" yaml:"url"`
	Headers map[string]string `json:"headers" yaml:"headers"`
	Body    string            `json:"body" yaml:"body"`
}

type Collection struct {
	Name     string    `json:"name" yaml:"name"`
	Requests []Request `json:"requests" yaml:"requests"`
}

func DefaultCollections() []Collection {
	return []Collection{{Name: "General API Tests", Requests: []Request{
		{Name: "Fetch Sample Post", Method: "GET", URL: "https://jsonplaceholder.typicode.com/posts/1", Headers: map[string]string{"Accept": "application/json"}},
		{Name: "Create Post (JSON)", Method: "POST", URL: "https://jsonplaceholder.typicode.com/posts", Headers: map[string]string{"Content-Type": "application/json"}, Body: "{\n  \"title\": \"goman Test\",\n  \"body\": \"Fast TUI API client\",\n  \"userId\": 42\n}"},
		{Name: "GitHub Public User", Method: "GET", URL: "https://api.github.com/users/octocat", Headers: map[string]string{"User-Agent": "goman"}},
	}}}
}

type Storage struct{ Dir string }

func LoadWorkspace(workspace string) (Storage, []Collection, error) {
	store := Storage{Dir: filepath.Join(workspace, ".goman")}
	for _, directory := range []string{store.Dir, filepath.Join(workspace, ".ghostman")} {
		for _, format := range []string{"yaml", "json"} {
			_, err := os.Stat(filepath.Join(directory, "collections."+format))
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			if err != nil {
				return store, nil, err
			}
			collections, err := (Storage{Dir: directory}).Load()
			if err == nil && directory != store.Dir {
				err = store.Save(collections)
			}
			return store, collections, err
		}
	}
	collections, err := store.Load()
	return store, collections, err
}

func (s Storage) Load() ([]Collection, error) {
	var failures []error
	found := false
	for _, format := range []string{"yaml", "json"} {
		data, err := os.ReadFile(filepath.Join(s.Dir, "collections."+format))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		found = true
		var collections []Collection
		if err == nil {
			if format == "yaml" {
				err = yaml.Unmarshal(data, &collections)
			} else {
				err = json.Unmarshal(data, &collections)
			}
		}
		if err == nil && collections != nil {
			return collections, nil
		}
		if err == nil {
			err = errors.New("expected a collection list")
		}
		failures = append(failures, fmt.Errorf("loading collections.%s: %w", format, err))
	}
	if found {
		return nil, errors.Join(failures...)
	}
	collections := DefaultCollections()
	return collections, s.Save(collections)
}

func (s Storage) Save(collections []Collection) error {
	if collections == nil {
		collections = []Collection{}
	}
	jsonData, err := json.MarshalIndent(collections, "", "  ")
	if err != nil {
		return err
	}
	yamlData, err := yaml.Marshal(collections)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(s.Dir, 0700); err != nil {
		return err
	}
	for _, file := range []struct {
		name string
		data []byte
	}{{"collections.yaml", yamlData}, {"collections.json", jsonData}} {
		if err := atomicWrite(filepath.Join(s.Dir, file.name), file.data); err != nil {
			return fmt.Errorf("saving %s: %w", file.name, err)
		}
	}
	return nil
}

func atomicWrite(path string, data []byte) error {
	file, err := os.CreateTemp(filepath.Dir(path), ".collections-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if _, err := file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), path)
}
