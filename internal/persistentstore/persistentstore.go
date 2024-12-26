package persistentstore

import (
	"fmt"
	"log"
	"os"

	"gopkg.in/yaml.v2"
)

// This package implements a persistent map, which is preserved across invocations in a YAML file.
// It is intended to be reasonably generic.
// The key needs to be a comparable type (as the underlying representation is a map).
// The stored data can be any type.

// The Store type records the persistent data  and tracks whether the data has been modified
type Store[K comparable, T any] struct {
	Active bool    // True if the cache is in use
	Dirty  bool    // True if the cache has been modified (and should be written out)
	Data   map[K]T // A cache of key => stored-data
}

// Initialises the persistent store from a YAML file (with presumably appropriate data).
// If the YAML file does not exist, it may optionally be created.
// Data from the file is unmarshalled into the store.
//
// On successful exit a pointer to the store and a nil error are returned.
func (Store[K, T]) Init(storeFilename string, createIfMissing bool, verbose bool) (*Store[K, T], error) {
	store := new(Store[K, T])
	store.Active = false
	store.Dirty = false
	store.Data = make(map[K]T)
	if storeFilename != "" {
		file, err := os.ReadFile(storeFilename)
		if err != nil {
			if os.IsNotExist(err) {
				if createIfMissing {
					newFile, err := os.Create(storeFilename)
					if err != nil {
						// Store file does not exist and cannot be created
						return store, err
					}
					newFile.Close()
					fmt.Printf("Created empty store file: %s\n", storeFilename)
					file, err = os.ReadFile(storeFilename)
					if err != nil {
						// Store file created but cannot be read
						return store, err
					}
				} else {
					// An error has occurred that is something other than the specified file not existing
					return store, err
				}
			}
		}
		store.Active = true
		// Read the existing cache YAML data into the cache
		err = yaml.Unmarshal(file, store.Data)
		if err != nil {
			if verbose {
				fmt.Println("persistentstore: failed to unmarshal")
			}
			return store, err
		}
	}

	if verbose {
		fmt.Printf("Initial number of store entries: %d, store address: %p\n", len(store.Data), store)
	}
	return store, nil
}

// Performs a lookup in the store and retrieves the data (if any) stored against the given key.
// The return mimics that returned by a map, i.e. the value and a boolean true if the key exists.
func (thing *Store[K, T]) Lookup(key K) (T, bool) {
	value, found := thing.Data[key]
	return value, found
}

// Returns true if the store has been modified and false otherwise
func (thing *Store[K, T]) IsModified() bool {
	return thing.Dirty
}

// Updates the data stored against the specified key.
//
// Note that this update happens even if there is already data stored against the specified key.
func (thing *Store[K, T]) Update(key K, data T) {
	thing.Data[key] = data
	thing.Dirty = true
}

// Save the stored data, if it has changed.
//
// Data is stored as YAML in the specified file.
func (thing *Store[K, T]) Save(filename string) {
	if thing.Active && thing.Dirty {
		fmt.Println("Writing **new** Store")
		data, err := yaml.Marshal(thing.Data)
		if err != nil {
			log.Fatal("Bad Store.Data: ", err)
		}
		err = os.WriteFile(filename, data, 0644)
		if err != nil {
			log.Fatal("Failed Store.Data write: ", err)
		}
	}

}
