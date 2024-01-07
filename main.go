package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	ffmpeg_go "github.com/u2takey/ffmpeg-go"
)

var AUDIO_EXT = []string{
	"mp3",
	"aac",
	"flac",
	"wav",
	"m4a",
	"dsf",
	"dff",
	"alac",
	"wma",
}

// it must be a subset of AUDIO_EXT
var COMPRESSED_AUDIO_EXT = []string{
	"mp3",
	"aac",
	"m4a",
	"wma",
}

var NON_AUDIO_EXT = []string{
	"jpg",
	"png",
	"webm",
	"ini",
	"txt",
	"db",
	"torrent",
	"cue",
	"ds_store",
	"m3u",
	"log",
}

var DIR = `E:\Favourite Music`
var OUTDIR = `E:\AAC_GO`
var NUM_THREAD = 28

func main() {
	println(`
	________________________ 
	|  ____________________  |
	| | MusicLibTranscoder | |
	| |____________________| |
	|________________________|`)

	// CLI args
	flag_ow := flag.Bool("overwrite", false, "silently overwrite exist files.")
	flag.Parse()

	println("Walking the library...")

	var files []string

	// counter := 0
	filepath.WalkDir(DIR, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			fmt.Printf("Error when walking into %s (%s): %s\n", path, d.Name(), err)
		}
		// fmt.Printf("%s isDir=%s\n", path, strconv.FormatBool(d.IsDir()))
		// counter++

		// skip dirs
		if d.IsDir() {
			return nil
		}

		// get the extension
		ext := strings.ToLower(filepath.Ext(path))

		// for audio files, insert to list
		if isInList(AUDIO_EXT, ext) {
			files = append(files, path)
		} else if !isInList(NON_AUDIO_EXT, ext) {
			println("WARNING: new file type detected:", ext)
		}

		return nil
	})
	println("Walking the library...", len(files))

	/* --------------------------------- WORKER --------------------------------- */

	// de-dup
	// t := make(map[string]int)
	// for _, v := range files {
	// 	outfile := strings.Replace(filepath.Base(v), filepath.Ext(v), ".aac", 1)
	// 	t[outfile]++
	// }
	// for k, v := range t {
	// 	if v > 1 {
	// 		println(k, v)
	// 	}
	// }
	// return

	var guard *sync.Mutex = new(sync.Mutex)
	var top *int = new(int)
	*top = 0
	var wg *sync.WaitGroup = new(sync.WaitGroup)
	var report_chan chan int = make(chan int)
	var done_counter int = 0
	var abort_chans []chan bool = make([]chan bool, NUM_THREAD)

	for i := 0; i < NUM_THREAD; i++ {
		abort_chans[i] = make(chan bool)
		wg.Add(1)
		go worker(files, top, guard, report_chan, abort_chans[1], wg, *flag_ow)
	}

	start_time := time.Now().Unix()

	for {
		<-report_chan
		done_counter++
		current_time := time.Now().Unix()
		speed := float32(done_counter) / float32(current_time-start_time)
		eta := float32(len(files)-done_counter) / speed
		fmt.Printf("Processing...%d/%d   ETA: %3.0f minutes\r", done_counter, len(files), eta/60)
		if done_counter == len(files) {
			fmt.Printf("Processing...%d/%d   ETA: %3.0f minutes                \n", done_counter, len(files), eta/60)
			break
		}
	}

	wg.Wait()
	println("Done.")
}

func isInList(l []string, x string) bool {
	for _, v := range l {
		if "."+v == x {
			return true
		}
	}
	return false
}

func worker(files []string, top *int, mutex *sync.Mutex, report chan int, abort chan bool, wg *sync.WaitGroup, ow bool) {
	for {
		select {
		case <-abort:
			wg.Done()
			return
		default:
			mutex.Lock()
			if *top >= len(files) {
				wg.Done()
				mutex.Unlock()
				return
			}
			current := *top
			*top++
			mutex.Unlock()

			file := files[current]

			// copy if compress audio
			if isInList(COMPRESSED_AUDIO_EXT, filepath.Ext(file)) {
				outpath := filepath.Join(OUTDIR, filepath.Base(file))

				// check file exists
				_, err := os.Stat(outpath)
				if errors.Is(err, os.ErrNotExist) || ow {
					// open src file
					src, err := os.Open(file)
					if err != nil {
						println("Failed to open src file", file)
					}
					// create dst file
					dst, err := os.Create(outpath)
					if err != nil {
						println("Failed to create dst file for", file)
					}
					// copy
					_, err = io.Copy(dst, src)
					if err != nil {
						println("Failed to copy from", file)
					}

					dst.Sync()
					dst.Close()
					src.Close()
				}
			} else {
				outfile := strings.Replace(filepath.Base(file), filepath.Ext(file), ".aac", 1)
				outpath := filepath.Join(OUTDIR, outfile)

				// check file exists
				_, err := os.Stat(outpath)
				if errors.Is(err, os.ErrNotExist) || ow {
					err = ffmpeg_go.Input(file).Output(outpath, ffmpeg_go.KwArgs{"b:a": "320k"}).OverWriteOutput().Silent(true).Run()
					if err != nil {
						println(err.Error())
					}
				}

				// check file exists
				_, err = os.Stat(outpath)
				if errors.Is(err, os.ErrNotExist) {
					println("ERROR: Transcoding seems failed", file)
				}
			}

			report <- current
		}
	}
}
