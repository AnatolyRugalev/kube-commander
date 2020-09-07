package config

import (
	"context"
	"errors"
	"fmt"
	"github.com/fsnotify/fsnotify"
	"io/ioutil"
	"os"

	"github.com/AnatolyRugalev/kube-commander/pb"
	"google.golang.org/protobuf/encoding/protojson"
	"path/filepath"
	"sigs.k8s.io/yaml"
)

const DefaultName = ".kubecom.yaml"

func DefaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("error obtaining user home dir: %w", err)
	}
	return filepath.Join(home, DefaultName), nil
}

type Event struct {
	Config *pb.Config
	Err    error
}

func Exists(path string) bool {
	_, err := os.Stat(path)
	return !errors.Is(err, os.ErrNotExist)
}

func Init(path string) error {
	return ioutil.WriteFile(path, nil, 0755)
}

func Watch(ctx context.Context, path string, ch chan<- Event) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("could not start watch notifier: %w", err)
	}
	err = watcher.Add(path)
	if err != nil {
		return err
	}

	go func() {
		defer watcher.Close()
		config, err := Load(path)
		if err != nil {
			ch <- Event{Err: err}
		} else {
			ch <- Event{Config: config}
		}
		for {
			select {
			case <-ctx.Done():
				ch <- Event{Err: ctx.Err()}
				return
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				ch <- Event{Err: err}
				return
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				if event.Op&fsnotify.Write == fsnotify.Write {
					config, err := Load(path)
					if err != nil {
						ch <- Event{Err: err}
					} else {
						ch <- Event{Config: config}
					}
				}
			}
		}
	}()
	return nil
}

func Save(path string, config *pb.Config) error {
	err := os.MkdirAll(filepath.Dir(path), 0755)
	if err != nil {
		return fmt.Errorf("could not create configuration file directory")
	}
	jsonB, err := protojson.Marshal(config)
	if err != nil {
		return fmt.Errorf("error marshalling config: %w", err)
	}
	yamlB, err := yaml.JSONToYAML(jsonB)
	if err != nil {
		return fmt.Errorf("error converting configuration to YAML: %w", err)
	}
	err = ioutil.WriteFile(path, yamlB, 0755)
	if err != nil {
		return fmt.Errorf("error writing config file: %w", err)
	}
	return nil
}

func Load(path string) (*pb.Config, error) {
	yamlB, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("error reading configuration file: %w", err)
	}
	config := pb.Config{}
	if len(yamlB) == 0 {
		return &config, nil
	}
	jsonB, err := yaml.YAMLToJSON(yamlB)
	if err != nil {
		return nil, fmt.Errorf("error converting configuration to JSON: %w", err)
	}
	err = protojson.Unmarshal(jsonB, &config)
	if err != nil {
		return nil, fmt.Errorf("error unmarshalling configuration: %w", err)
	}
	return &config, nil
}
