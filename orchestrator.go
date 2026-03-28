package main

import (
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/vbauerster/mpb/v8"
	"github.com/vbauerster/mpb/v8/decor"
)

/*
StartOrchestrator initializes the file watcher and processes files in the input folder.

This function performs the following tasks:

1. **Count Total Files**:
   - Recursively walks through the input folder to count the total number of files.
   - This count is used to initialize a progress bar for tracking the processing status.

2. **Initialize Progress Bar**:
   - Uses the `mpb` library to display a CLI progress bar.
   - The progress bar updates as each file is processed.

3. **Process Existing Files**:
   - Recursively walks through the input folder to process each file.
   - Before processing, it checks if the destination file already exists in the output folder.
     - If the file exists, it skips processing and increments the progress bar.
   - If the file does not exist, it processes the file (e.g., transcodes audio or copies non-audio files).

4. **Handle Album Artwork**:
   - During the transcode process, the function ensures that album artwork (if present) is correctly mapped to the output file.
   - Uses FFmpeg's `-map` option with the `?` modifier to conditionally include the artwork.
   - If the input file contains an MJPEG stream (commonly used for embedded album artwork), it is copied to the output file without re-encoding.
   - If no artwork is present, the process continues without errors.

5. **Initialize File Watcher**:
   - Sets up a file watcher using `fsnotify` to monitor the input folder for new files.
   - When a new file is created, it checks if the destination file already exists.
     - If the file exists, it skips processing.
     - Otherwise, it processes the new file.

6. **Add Input Folder to Watcher**:
   - Adds the input folder and its subdirectories to the file watcher.
   - Ensures that new files in any subdirectory are also monitored.

7. **Block Forever**:
   - The function blocks indefinitely to keep the file watcher running.
   - This ensures that the application continues to monitor the input folder for new files.

This function is designed to handle large-scale file processing efficiently, with features like progress tracking, conditional processing, and support for embedded album artwork. It ensures that files are not reprocessed unnecessarily and provides a user-friendly CLI experience.
*/

const (
	watchEventBuffer = 2048
	processQueueSize = 2048
	workerCount      = 2

	debounceWindow = 2 * time.Second
	debounceTick   = 250 * time.Millisecond

	stablePollInterval = 500 * time.Millisecond
	stableWindow       = 2 * time.Second
	stableTimeout      = 2 * time.Minute
)

// StartOrchestrator initializes the file watcher and processes files in the input folder.
func StartOrchestrator(inputFolder, outputFolder string, dryRun, overwrite bool) {
	// Count total files
	totalFiles := 0
	err := filepath.Walk(inputFolder, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			totalFiles++
		}
		return nil
	})
	if err != nil {
		log.Fatalf("Failed to count files: %v", err)
	}

	// Initialize progress bar
	p := mpb.New()
	bar := p.AddBar(int64(totalFiles),
		mpb.PrependDecorators(
			decor.Name("Processing: "),
			decor.CountersNoUnit("%d/%d"),
		),
		mpb.AppendDecorators(
			decor.Percentage(),
		),
	)

	// Process existing files with progress.
	// Destination skip/overwrite checks are done by processFile with canonical output paths.
	err = filepath.Walk(inputFolder, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			processFile(path, inputFolder, outputFolder, dryRun, overwrite)
			bar.Increment()
		}
		return nil
	})
	if err != nil {
		log.Fatalf("Failed to process files: %v", err)
	}

	// Wait for progress bar to complete
	p.Wait()

	// Initialize file watcher
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatalf("Failed to initialize watcher: %v", err)
	}
	defer watcher.Close()

	rawEvents := make(chan string, watchEventBuffer)
	debouncedEvents := make(chan string, processQueueSize)

	startWorkers(debouncedEvents, inputFolder, outputFolder, dryRun, overwrite)
	startDebouncer(rawEvents, debouncedEvents)

	// Start watching the input folder
	go func() {
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					close(rawEvents)
					return
				}

				if event.Op&(fsnotify.Create|fsnotify.Write|fsnotify.Rename) == 0 {
					continue
				}

				if shouldWatchAsDirectory(watcher, event.Name) {
					continue
				}

				select {
				case rawEvents <- event.Name:
				default:
					log.Printf("Dropping watch event due to full buffer: %s", event.Name)
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				log.Printf("Watcher error: %v", err)
			}
		}
	}()

	// Add input folder to watcher
	err = addDirRecursiveToWatcher(watcher, inputFolder)
	if err != nil {
		log.Fatalf("Failed to watch input folder: %v", err)
	}

	// Block forever
	select {}
}

func addDirRecursiveToWatcher(watcher *fsnotify.Watcher, root string) error {
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if err := watcher.Add(path); err != nil {
				return err
			}
		}
		return nil
	})
}

func shouldWatchAsDirectory(watcher *fsnotify.Watcher, path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}

	if !info.IsDir() {
		return false
	}

	if err := addDirRecursiveToWatcher(watcher, path); err != nil {
		log.Printf("Failed to add newly created directory to watcher (%s): %v", path, err)
	}
	return true
}

func startDebouncer(in <-chan string, out chan<- string) {
	go func() {
		pending := make(map[string]time.Time)
		ticker := time.NewTicker(debounceTick)
		defer ticker.Stop()

		for {
			select {
			case path, ok := <-in:
				if !ok {
					for p := range pending {
						out <- p
					}
					close(out)
					return
				}
				pending[path] = time.Now()

			case <-ticker.C:
				now := time.Now()
				for p, seen := range pending {
					if now.Sub(seen) < debounceWindow {
						continue
					}

					select {
					case out <- p:
						delete(pending, p)
					default:
						// Keep pending for the next tick if workers are back-pressured.
					}
				}
			}
		}
	}()
}

func startWorkers(queue <-chan string, inputFolder, outputFolder string, dryRun, overwrite bool) {
	for i := 0; i < workerCount; i++ {
		go func() {
			for path := range queue {
				info, err := os.Stat(path)
				if err != nil {
					continue
				}
				if info.IsDir() {
					continue
				}

				if !waitForStableFile(path, stablePollInterval, stableWindow, stableTimeout) {
					log.Printf("Skipping unstable file after timeout: %s", path)
					continue
				}

				processFile(path, inputFolder, outputFolder, dryRun, overwrite)
			}
		}()
	}
}
