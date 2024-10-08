package lib

import (
	"fmt"
	"github.com/friendsofgo/errors"
	"io/fs"
	"math"
	"os"
	"path"
	"regexp"
	"slices"
	"strconv"
	"time"
)

type FileMapping struct {
	LayerIdx   int
	SourcePath string
	TargetPath string
	FileMode   fs.FileMode
	Modified   time.Time
	Size       int64
}

type FileCollision struct {
	Path                  string
	CollidingLayerIndexes []int
}

type EntryPathTypeCollision struct {
	Path                  string
	DirectoryLayerIndexes []int
	FileLayerIndexes      []int
}

// MergeDirectoryLayers merges multiple layers of directories.
// returns a list of file mappings for each input layer (same slice size as input)
func MergeDirectoryLayers(layers []fs.FS, basePath string) (fileMappings []FileMapping, fileCollisions []FileCollision, typeCollisions []EntryPathTypeCollision, err error) {
	entriesByNormalizedName := map[normalizedEntryName][]layerEntry{}
	for layerIdx, layer := range layers {
		entries, err := fs.ReadDir(layer, basePath)
		if err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				return nil, nil, nil, errors.Wrapf(err, "failed to read directory \"%s\" for layer %d", basePath, layerIdx+1)
			}
		}

		for _, entry := range entries {
			entryName := entry.Name()
			isDir := entry.IsDir()
			sourcePath := path.Join(basePath, entryName)
			normalizedName, targetName, orderingIndex, entryOrderingType := normalizeEntryName(entryName, isDir)

			entryInfo, err := entry.Info()
			if err != nil {
				return nil, nil, nil, errors.Wrapf(err, `failed to get file info for "%s" in layer %d`, sourcePath, layerIdx+1)
			}

			newEntry := layerEntry{
				sourcePath:    sourcePath,
				targetName:    targetName,
				layerIdx:      layerIdx,
				isDir:         isDir,
				orderingIndex: orderingIndex,
				orderingType:  entryOrderingType,
				fileMode:      entryInfo.Mode(),
				modified:      entryInfo.ModTime(),
				size:          entryInfo.Size(),
			}

			existingEntries, ok := entriesByNormalizedName[normalizedName]
			if !ok {
				existingEntries = []layerEntry{newEntry}
			} else {
				existingEntries = append(existingEntries, newEntry)
			}
			entriesByNormalizedName[normalizedName] = existingEntries
		}
	}

	for normalizedName, entries := range entriesByNormalizedName {
		normalizedFullPath := path.Join(basePath, normalizedName.name)

		dirIndexes, fileIndexes := checkPathEntryTypes(entries)
		if len(dirIndexes) == 0 {
			// all files -> merge files
			subFileMappings, collisionLayerIdxs := mergeFileEntries(entries)
			fileMappings = append(fileMappings, subFileMappings...)
			if len(collisionLayerIdxs) > 0 {
				fileCollisions = append(fileCollisions, FileCollision{
					Path:                  normalizedFullPath,
					CollidingLayerIndexes: collisionLayerIdxs,
				})
			}
		} else if len(fileIndexes) == 0 {
			if normalizedName.ordered {
				panic("programming error: directories should never be ordered")
			}
			// all directories -> merge directory
			subFileMappings, subFileCollisions, subTypeCollisions, err := MergeDirectoryLayers(layers, normalizedFullPath)
			if err != nil {
				return nil, nil, nil, errors.Wrapf(err, "failed to merge sub directory %s", normalizedFullPath)
			}
			fileMappings = append(fileMappings, subFileMappings...)
			fileCollisions = append(fileCollisions, subFileCollisions...)
			typeCollisions = append(typeCollisions, subTypeCollisions...)
		} else {
			// mixed: report error
			typeCollisions = append(typeCollisions, EntryPathTypeCollision{
				Path:                  normalizedFullPath,
				DirectoryLayerIndexes: dirIndexes,
				FileLayerIndexes:      fileIndexes,
			})
		}
	}

	return
}

type layerEntry struct {
	sourcePath    string
	targetName    string
	layerIdx      int
	isDir         bool
	orderingIndex int
	orderingType  orderingType
	fileMode      fs.FileMode
	modified      time.Time
	size          int64
}

func (l layerEntry) toFileMapping(targetPath string) FileMapping {
	return FileMapping{
		LayerIdx:   l.layerIdx,
		SourcePath: l.sourcePath,
		TargetPath: targetPath,
		FileMode:   l.fileMode,
		Modified:   l.modified,
		Size:       l.size,
	}
}

var orderedEntryNameRegex = regexp.MustCompile(`^(\d\d\d)(_(pre|post))?_(.*)$`)

type orderingType int

const (
	unordered             orderingType = iota
	preOrderingPreference              = iota
	ordered
	postOrderingPreference
)

type normalizedEntryName struct {
	ordered bool
	name    string // empty if ordered = true
}

// normalizeEntryName normalizes the name of a DirEntry
// targetName: same as normalized.name if not an ordered file
func normalizeEntryName(name string, isDir bool) (normalized normalizedEntryName, targetName string, index int, orderingType orderingType) {
	if isDir {
		return normalizedEntryName{
			ordered: false,
			name:    name,
		}, name, 0, unordered
	}

	subMatches := orderedEntryNameRegex.FindStringSubmatch(name)
	switch len(subMatches) {
	case 0:
		return normalizedEntryName{
			ordered: false,
			name:    name,
		}, name, 0, unordered
	case 5:
		var err error
		index, err = strconv.Atoi(subMatches[1])
		if err != nil {
			panic(fmt.Sprintf("failed to parse index %s: %v", subMatches[1], err))
		}
		orderingType, err = orderingPreferenceFromString(subMatches[3])
		if err != nil {
			panic("failed to parse ordering type: " + err.Error())
		}
		return normalizedEntryName{
			ordered: true,
		}, subMatches[4], index, orderingType
	default:
		panic("expected 0 or 5 sub matches for " + name)
	}
}

func orderingPreferenceFromString(v string) (orderingType, error) {
	switch v {
	case "":
		return ordered, nil
	case "pre":
		return preOrderingPreference, nil
	case "post":
		return postOrderingPreference, nil
	default:
		return 0, errors.Errorf("unexpected preference string %s", v)
	}
}

// checkPathEntryTypes returns the layer indexes that have a directory entry or indexes that have file entries
func checkPathEntryTypes(entries []layerEntry) (dirIndexes []int, fileIndexes []int) {
	for _, entry := range entries {
		if entry.isDir {
			dirIndexes = append(dirIndexes, entry.layerIdx)
		} else {
			fileIndexes = append(fileIndexes, entry.layerIdx)
		}
	}
	return
}

// mergeFiles creates the FileMapping for all the given entries.
// this function assumes that all entries:
//   - are files
//   - have the same normalized paths (and thus either all ordered or all unordered)
//
// returns either the same number of mappings as input entries OR a list of colliding layer indexes
func mergeFileEntries(entries []layerEntry) (mappings []FileMapping, collidingLayerIndexes []int) {
	if len(entries) == 0 {
		return []FileMapping{}, nil
	}

	var unorderedCount int
	layerIndexes := make([]int, len(entries))
	var preEntries, neutralEntries, postEntries []layerEntry
	for i, entry := range entries {
		switch entry.orderingType {
		case unordered:
			unorderedCount++
		case preOrderingPreference:
			preEntries = append(preEntries, entry)
		case ordered:
			neutralEntries = append(neutralEntries, entry)
		case postOrderingPreference:
			postEntries = append(postEntries, entry)
		}

		layerIndexes[i] = entry.layerIdx
	}

	orderedCount := len(entries) - unorderedCount
	if (orderedCount > 0) == (unorderedCount > 0) {
		panic("programming error: entries must either all be ordered or unordered")
	}

	// handle only-unordered files case
	if unorderedCount > 0 {
		if len(entries) > 1 {
			return nil, layerIndexes
		}
		onlyEntry := entries[0]
		return []FileMapping{onlyEntry.toFileMapping(onlyEntry.sourcePath)}, nil
	}

	mappings = make([]FileMapping, 0, len(entries))
	indexLength := uint(math.Max(float64(intBase10Length(len(entries))), 3))

	for _, layerEntries := range [][]layerEntry{preEntries, neutralEntries, postEntries} {
		mappings = append(mappings, mapOrderedFiles(layerEntries, len(mappings), indexLength)...)
	}

	return mappings, nil
}

func intBase10Length(number int) uint {
	if number == 0 {
		return 1
	}
	if number < 0 {
		return uint(math.Log10(-float64(number))) + 1 + 1
	}
	return uint(math.Log10(float64(number))) + 1
}

// mapOrderedFiles creates the file mappings for colliding ordered entries
// this function will sort the entries slice in place
func mapOrderedFiles(entries []layerEntry, orderIndexStart int, indexLength uint) []FileMapping {
	// sort, first by layerIdx, then by ordering index
	slices.SortFunc(entries, func(a, b layerEntry) int {
		if a.layerIdx == b.layerIdx {
			return a.orderingIndex - b.orderingIndex
		}
		return a.layerIdx - b.layerIdx
	})

	filenameFormat := fmt.Sprintf("%%0%dd_%%s", indexLength)
	mappings := make([]FileMapping, len(entries))

	for i, entry := range entries {
		orderIndex := orderIndexStart + i
		dirPath := path.Dir(entry.sourcePath)
		filename := fmt.Sprintf(filenameFormat, orderIndex, entry.targetName)

		mappings[i] = entry.toFileMapping(path.Join(dirPath, filename))
	}
	return mappings
}
