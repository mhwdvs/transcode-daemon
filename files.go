package main

import (
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// processFile processes a file by either transcoding audio files or skipping non-audio files.
func processFile(filePath, inputFolder, outputFolder string, dryRun, overwrite bool) {
	info, err := os.Stat(filePath)
	if err != nil {
		log.Printf("Skipping path %s: %v", filePath, err)
		return
	}
	if info.IsDir() {
		return
	}

	outputPath, err := resolveOutputPath(filePath, inputFolder, outputFolder)
	if err != nil {
		log.Printf("Skipping file %s: %v", filePath, err)
		return
	}

	if !overwrite {
		if _, err := os.Stat(outputPath); err == nil {
			log.Printf("Skipping file %s: destination already exists at %s", filePath, outputPath)
			return
		} else if !os.IsNotExist(err) {
			log.Printf("Skipping file %s: failed to check destination %s: %v", filePath, outputPath, err)
			return
		}
	}

	if isAudioFile(filePath) {
		if dryRun {
			log.Printf("[Dry-Run] Transcoding audio file: %s -> %s", filePath, outputPath)
		} else {
			transcodeAudio(filePath, inputFolder, outputFolder, overwrite)
		}
	} else if isVideoFile(filePath) {
		log.Printf("Skipping video file: %s", filePath)
	} else {
		if dryRun {
			log.Printf("[Dry-Run] Copying non-audio file: %s -> %s", filePath, outputPath)
		} else {
			copyFile(filePath, inputFolder, outputFolder, overwrite)
		}
	}
}

// resolveOutputPath maps an input file to its canonical destination path.
// Audio files are mapped to .m4a in the same relative directory structure.
func resolveOutputPath(filePath, inputFolder, outputFolder string) (string, error) {
	relPath, err := filepath.Rel(inputFolder, filePath)
	if err != nil {
		return "", err
	}

	relPath = filepath.Clean(relPath)
	if relPath == ".." || strings.HasPrefix(relPath, ".."+string(filepath.Separator)) {
		return "", os.ErrPermission
	}

	outputPath := filepath.Join(outputFolder, relPath)
	if isAudioFile(filePath) {
		outputPath = strings.TrimSuffix(outputPath, filepath.Ext(outputPath)) + ".m4a"
	}

	return outputPath, nil
}

// isAudioFile checks if a file is an audio file based on its extension.
func isAudioFile(filePath string) bool {
	audioExtensions := []string{".mp3", ".flac", ".wav", ".ogg", ".aac"}
	ext := strings.ToLower(filepath.Ext(filePath))
	for _, audioExt := range audioExtensions {
		if ext == audioExt {
			return true
		}
	}
	return false
}

// isVideoFile checks if a file is a video file based on its extension.
func isVideoFile(filePath string) bool {
	videoExtensions := []string{".mp4", ".mkv", ".avi", ".mov", ".flv", ".wmv"}
	ext := strings.ToLower(filepath.Ext(filePath))
	for _, videoExt := range videoExtensions {
		if ext == videoExt {
			return true
		}
	}
	return false
}

// transcodeAudio transcodes an audio file to AAC 320kbps .m4a format.
//
// This function performs the following steps:
//
// 1. **Determine Relative Path**:
//   - Calculates the relative path of the input file with respect to the input folder.
//   - This ensures the output file maintains the same folder structure as the input.
//
// 2. **Create Output Directory**:
//   - Ensures that the output directory exists by creating it if necessary.
//
// 3. **Set Output File Path**:
//   - Constructs the output file path by replacing the input file's extension with `.m4a`.
//
// 4. **Handle Album Artwork**:
//   - Uses FFmpeg to conditionally include album artwork (if present) in the output file.
//   - The `-map 0:a` option ensures only audio streams are processed.
//   - The `-map 0:v:0?` option conditionally includes the first video stream (album artwork) if it exists.
//   - The `?` modifier prevents errors if no video stream is present.
//
// 5. **Transcode Audio**:
//   - Invokes FFmpeg to transcode the audio to AAC format at 320kbps.
//   - Logs detailed output and errors for debugging purposes.
func transcodeAudio(filePath, inputFolder, outputFolder string, overwrite bool) {
	outputPath, err := resolveOutputPath(filePath, inputFolder, outputFolder)
	if err != nil {
		log.Printf("Failed to determine output path for %s: %v", filePath, err)
		return
	}

	outputDir := filepath.Dir(outputPath)
	if err := os.MkdirAll(outputDir, os.ModePerm); err != nil {
		log.Printf("Failed to create output directory: %v", err)
		return
	}

	args := []string{}
	if overwrite {
		args = append(args, "-y")
	} else {
		args = append(args, "-n")
	}

	args = append(args,
		"-i", filePath,
		"-map", "0:a",
		"-map", "0:v:0?",
		"-c:a", "libfdk_aac",
		"-b:a", "320k",
		"-ar", "44100",
		"-af", "loudnorm=I=-16:TP=-1.5:LRA=11",
		"-c:v", "copy",
		"-movflags", "+faststart",
		outputPath,
	)

	cmd := exec.Command("ffmpeg", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("Failed to transcode file %s: %v\nOutput: %s", filePath, err, string(output))
	} else {
		log.Printf("Transcoded: %s -> %s", filePath, outputPath)
	}
}

// copyFile copies a non-audio file to the output folder while maintaining the folder structure.
func copyFile(filePath, inputFolder, outputFolder string, overwrite bool) {
	outputPath, err := resolveOutputPath(filePath, inputFolder, outputFolder)
	if err != nil {
		log.Printf("Failed to determine output path for %s: %v", filePath, err)
		return
	}
	outputDir := filepath.Dir(outputPath)

	if err := os.MkdirAll(outputDir, os.ModePerm); err != nil {
		log.Printf("Failed to create output directory: %v", err)
		return
	}

	inputFile, err := os.Open(filePath)
	if err != nil {
		log.Printf("Failed to open input file: %v", err)
		return
	}
	defer inputFile.Close()

	var outputFile *os.File
	if overwrite {
		outputFile, err = os.OpenFile(outputPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
		if err != nil {
			log.Printf("Failed to create output file: %v", err)
			return
		}
	} else {
		outputFile, err = os.OpenFile(outputPath, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o644)
		if err != nil {
			if os.IsExist(err) {
				log.Printf("Skipping file %s: destination already exists at %s", filePath, outputPath)
				return
			}
			log.Printf("Failed to create output file: %v", err)
			return
		}
	}
	defer outputFile.Close()

	if _, err := io.Copy(outputFile, inputFile); err != nil {
		log.Printf("Failed to copy file: %v", err)
	} else {
		log.Printf("Copied: %s -> %s", filePath, outputPath)
	}
}

// waitForStableFile returns true when file size and mtime are stable for stableWindow.
// It returns false when the timeout is reached.
func waitForStableFile(path string, pollInterval, stableWindow, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	var (
		lastSize   int64 = -1
		lastMod    time.Time
		stableFrom time.Time
	)

	for time.Now().Before(deadline) {
		info, err := os.Stat(path)
		if err != nil || info.IsDir() {
			time.Sleep(pollInterval)
			continue
		}

		currentSize := info.Size()
		currentMod := info.ModTime()

		if currentSize == lastSize && currentMod.Equal(lastMod) {
			if stableFrom.IsZero() {
				stableFrom = time.Now()
			}
			if time.Since(stableFrom) >= stableWindow {
				return true
			}
		} else {
			lastSize = currentSize
			lastMod = currentMod
			stableFrom = time.Now()
		}

		time.Sleep(pollInterval)
	}

	return false
}
