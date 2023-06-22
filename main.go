package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	// "github.com/ggerganov/whisper.cpp/bindings/go/pkg/whisper" //this imports the whisper package

	//this imports the youtube package
	"github.com/ggerganov/whisper.cpp/bindings/go/pkg/whisper"
	"github.com/go-audio/wav"
	"github.com/kkdai/youtube/v2"
	"github.com/timpratim/Speech-To-Text/models"
	"github.com/timpratim/Speech-To-Text/repository"
	"github.com/urfave/cli/v2" //this imports the cli package
)

const port = 8090

const dbUrl = "ws://192.168.29.102:8000/rpc"
const namespace = "surrealdb-conference-content"
const database = "yttranscriber"

const (
	srcUrl  = "https://huggingface.co/ggerganov/whisper.cpp/resolve/main" // The location of the models
	srcExt  = ".bin"                                                      // Filename extension
	bufSize = 1024 * 64                                                   // Size of the buffer used for downloading the model
)

var (
	// The models which will be downloaded, if no model is specified as an argument
	modelNames = []string{"ggml-tiny.en"} // "ggml-tiny", "ggml-base.en", "ggml-base", "ggml-small.en", "ggml-small", "ggml-medium.en", "ggml-medium", "ggml-large-v1", "ggml-large"}
)

// ContextForSignal returns a context object which is cancelled when a signal
// is received. It returns nil if no signal parameter is provided
func ContextForSignal(signals ...os.Signal) context.Context {
	if len(signals) == 0 {
		return nil
	}

	ch := make(chan os.Signal)
	ctx, cancel := context.WithCancel(context.Background())

	// Send message on channel when signal received
	signal.Notify(ch, signals...)

	// When any signal received, call cancel
	go func() {
		<-ch
		cancel()
	}()

	// Return success
	return ctx
}

func main() {
	// Create context which quits on SIGINT or SIGQUIT
	ctx := ContextForSignal(os.Interrupt, syscall.SIGQUIT)
	app := transcribe_app(ctx)

	err := app.Run(os.Args)
	if err != nil {
		fmt.Println(err)
	}
}

func transcribe_app(ctx context.Context) *cli.App {
	repository, err := repository.NewTranscriptionsRepository(dbUrl, "root", "root", namespace, database)
	if err != nil {
		log.Fatalf("failed to create shortener repository: %+v", err)
	}
	fmt.Println("Connected to database")
	// Close connections to the database at program shutdown
	// defer func() {
	// 	fmt.Println("Closing database")
	// 	repository.Close()
	// }()
	return &cli.App{
		Name:  "ytrans",
		Usage: "Transcribe YouTube videos",
		Commands: []*cli.Command{

			//get transcriptions by ytlink
			{
				Name:  "get",
				Usage: "Get transcriptions by ytlink",
				Action: func(c *cli.Context) error {
					ytLink := c.Args().First()
					if ytLink == "" {
						return fmt.Errorf("please provide a YouTube link")
					}
					transcriptions, err := repository.GerTranscriptionsByYtlink(ytLink)
					if err != nil {
						log.Println("Error: ", err)
					}
					fmt.Println(transcriptions)
					return nil

				},
			},
			// {
			// 	Name:  "save",
			// 	Usage: "Save transcription to database",
			// 	Action: func(c *cli.Context) error {
			// 		ytLink := c.Args().First()
			// 		if ytLink == "" {
			// 			return fmt.Errorf("please provide a YouTube link")
			// 		}

			// 		transcriptions := []models.Transcription{
			// 			{
			// 				Ytlink:    ytLink,
			// 				Index:     0,
			// 				Text:      "Hello",
			// 				StartTime: "00:00:00",
			// 				EndTime:   "00:00:00",
			// 			},
			// 		}

			// 		_, err := repository.SaveTranscriptions(ytLink, models.Transcriptions(transcriptions))
			// 		if err != nil {
			// 			log.Println("Error: ", err)
			// 		}

			// 		return nil

			// 	},
			// },

			{
				Name:  "link",
				Usage: "Transcribe a single YouTube link",
				Action: func(c *cli.Context) error {
					ytLink := c.Args().First()
					if ytLink == "" {
						return fmt.Errorf("please provide a YouTube link")
					}
					err := transcribe(ctx, ytLink, repository)
					if err != nil {
						log.Println("Error: ", err)
						return err

					}

					return nil
				},
			},
		},
	}
}

type ProgressWriter struct {
	writer        io.Writer
	totalBytes    int64
	contentLength int64
	onProgress    func(written int64, progressPercentage float64)
}

func (pw *ProgressWriter) Write(p []byte) (int, error) {
	n, err := pw.writer.Write(p)
	pw.totalBytes += int64(n)

	if pw.onProgress != nil {
		progressPercentage := float64(pw.totalBytes) / float64(pw.contentLength) * 100
		pw.onProgress(pw.totalBytes, progressPercentage)
	}

	return n, err
}

func NewProgressWriter(w io.Writer, contentLength int64, onProgress func(written int64, progressPercentage float64)) *ProgressWriter {
	return &ProgressWriter{
		writer:        w,
		totalBytes:    0,
		contentLength: contentLength,
		onProgress:    onProgress,
	}
}
func saveWav(ytLink, youtubeID string) (string, error) {
	// Initialize a YouTube client
	client := youtube.Client{}

	// Extract video details
	video, err := client.GetVideo(ytLink)
	if err != nil {
		return "", fmt.Errorf("failed to get video details: %w", err)
	}

	// Find the audio streamsx
	audioStream := video.Formats.FindByItag(251) // itag 251 is high quality opus audio

	//TODO: maybe optimize it using an io pipe?

	// Download the audio streams
	audioReader, _, err := client.GetStream(video, audioStream)
	if err != nil {
		return "", fmt.Errorf("failed to get audio stream: %w", err)
	}

	log.Println("Successfully downloaded the audio stream")

	outputFilename := fmt.Sprintf("%s_corrupted.wav", youtubeID)
	// Initialize the output wav file
	outputFile, err := os.Create(fmt.Sprintf("/data/%s", outputFilename))
	if err != nil {
		return "", fmt.Errorf("failed to create output file: %w", err)
	}
	defer outputFile.Close()
	log.Println("Successfully created the output file")
	audioSize := audioStream.ContentLength
	progressWriter := NewProgressWriter(outputFile, audioSize, func(written int64, progressPercentage float64) {
		fmt.Printf("Written %d bytes (%.2f%%)\n", written, progressPercentage)
	})

	// Use ffmpeg to convert the audio to wav format + resample for whisper
	cmd := exec.Command("ffmpeg", "-i", "pipe:0",
		// remove  header

		"-vn", "-ac", "1", "-ar", "16000", "-codec:a", "pcm_s16le", "-f", "wav", "pipe:1")
	cmd.Stdin = audioReader

	// Write the converted audio to the output file with progress tracking
	cmd.Stdout = progressWriter
	cmd.Stderr = os.Stderr
	err = cmd.Run()

	if err != nil {
		return "", fmt.Errorf("ffmpeg conversion failed: %w", err)
	}

	return outputFilename, nil
}

func transcribe(ctx context.Context, ytLink string, repository *repository.TranscriptionsRepository) error {

	//watch?v=QRZ_l7cVzzU
	//check if file exists
	// audioFilename := "jfk.wav"
	audioFilename, err := youtubeDL(ytLink)
	if err != nil {
		return fmt.Errorf("failed to download the video: %w", err)
	}

	log.Println("Downloading the model...")

	// Progress filehandle
	progress := os.Stdout

	progress, err = os.Open(os.DevNull)
	if err != nil {
		return fmt.Errorf("failed to open progress file: %w", err)
	}
	defer progress.Close()

	// Get output path
	out, err := GetOut()
	if err != nil {
		return fmt.Errorf("failed to get output path: %w", err)
	}

	// TODO: allow user to choose model
	var modelPath string
	// Download models - exit on error or interrupt
	for _, model := range GetModels() {
		url, err := URLForModel(model)
		if err != nil {
			fmt.Fprintln(os.Stderr, "Error:", err)
			continue
		} else if path, err := Download(ctx, progress, url, out); err == nil || err == io.EOF {
			modelPath = path
			continue
		} else if err == context.Canceled {
			os.Remove(path)
			fmt.Fprintln(progress, "\nInterrupted")
			break
		} else if err == context.DeadlineExceeded {
			os.Remove(path)
			fmt.Fprintln(progress, "Timeout downloading model")
			continue
		} else {
			os.Remove(path)
			fmt.Fprintln(os.Stderr, "Error:", err)
			break
		}
	}

	model, err := whisper.New(modelPath)
	if err != nil {
		return fmt.Errorf("failed to load model: %w", err)
	}
	defer model.Close()

	log.Println("Successfully loaded the model")

	// Create processing context
	context, err := model.NewContext()
	if err != nil {
		return fmt.Errorf("failed to create context: %w", err)
	}

	var data []float32
	// Decode the WAV file - load the full buffer
	data, err = decodePCMBuffer(audioFilename, data)
	if err != nil {
		return fmt.Errorf("failed to decode audio file: %w", err)
	}
	dataLen := len(data)
	//print data len
	log.Println("Audio data length: ", dataLen)
	// if data len is 0 apply ffmpeg
	if dataLen == 0 {
		// JzPfMbG1vrE_corrupted.wav  -> JzPfMbG1vrE.wav
		newFilename := strings.Replace(audioFilename, "_corrupted", "", 1)
		// ffmpeg -i JzPfMbG1vrE.wav -ar 16000 JzPfMbG1vrE.wav -y
		cmd := exec.Command("ffmpeg", "-i", fmt.Sprintf("/data/%s", audioFilename), "-ar", "16000", fmt.Sprintf("/data/%s", newFilename), "-y")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		err = cmd.Run()
		if err != nil {
			return fmt.Errorf("ffmpeg conversion failed: %w", err)
		}

		data, err = decodePCMBuffer(newFilename, data)
		if err != nil {
			return fmt.Errorf("failed to decode audio file again: %w", err)
		}
		dataLen = len(data)
		fmt.Println("Successfully converted the audio file ", dataLen)

	}

	fmt.Println("Starting the transcription...")
	// Segment callback when -tokens is specified
	var cb whisper.SegmentCallback

	if err := context.Process(data, cb); err != nil {
		return err
	}

	// Print out the results
	transcriptions, err := OutputSRT(context)
	if err != nil {
		return fmt.Errorf("failed to output SRT: %w", err)
	}

	// print got transcriptions
	log.Println("Got raw transcriptions: %v", transcriptions)

	_, err = repository.SaveTranscriptions(ytLink, models.ToModel(models.RawTranscriptions(transcriptions)))
	if err != nil {
		log.Println("Error: ", err)
	}

	return nil
}

func decodePCMBuffer(audioFilename string, data []float32) ([]float32, error) {
	fh, err := os.Open(fmt.Sprintf("/data/%s", audioFilename))

	if err != nil {
		return nil, err
	}
	defer fh.Close()
	dec := wav.NewDecoder(fh)
	if buf, err := dec.FullPCMBuffer(); err != nil {
		return nil, err
	} else if dec.SampleRate != whisper.SampleRate {
		return nil, fmt.Errorf("unsupported sample rate: %d", dec.SampleRate)
	} else if dec.NumChans != 1 {
		return nil, fmt.Errorf("unsupported number of channels: %d", dec.NumChans)
	} else {
		data = buf.AsFloat32Buffer().Data
	}
	return data, nil
}

func youtubeDL(ytLink string) (string, error) {
	log.Println("Downloading and converting YouTube link: ", ytLink)

	watch := strings.Split(ytLink, "/")[len(strings.Split(ytLink, "/"))-1]

	youtubeID := strings.Split(watch, "=")[len(strings.Split(watch, "="))-1]
	audioFilename := youtubeID + ".wav"

	_, err := os.Stat(fmt.Sprintf("/data/%s", audioFilename))
	if err == nil {
		log.Println("Audio file already exists, skipping download")

	} else {
		log.Println("Audio file does not exist, downloading")
		audioFilename, err = saveWav(ytLink, youtubeID)
		if err != nil {
			return "", fmt.Errorf("failed to save wav file: %w", err)
		}

		log.Println("Successfully saved the wav file as: output.wav")
	}
	return audioFilename, nil
}

func GetOut() (string, error) {
	// Get the current working directory
	wd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to get working directory: %w", err)
	}
	return wd, nil
}

// GetModels returns the list of models to download
func GetModels() []string {
	return modelNames
}

// Output text as SRT file
func OutputSRT(context whisper.Context) (*[]models.RawTranscription, error) {
	n := 1
	results := make([]models.RawTranscription, 0)

	for {
		segment, err := context.NextSegment()
		if err != nil {
			break
		}
		transcription := models.RawTranscription{
			StartTs: segment.Start,
			StopTs:  segment.End,
			Text:    segment.Text,
			Index:   n,
		}
		fmt.Println(srtTimestamp(segment.Start), "-->", srtTimestamp(segment.End))
		fmt.Println(segment.Text)
		fmt.Println("n: ", n)
		results = append(results, transcription)
		n++
	}
	return &results, nil
}

// Output text to terminal
func Output(w io.Writer, context whisper.Context, colorize bool) error {
	for {
		segment, err := context.NextSegment()
		if err == io.EOF {
			return nil
		} else if err != nil {
			return err
		}
		fmt.Fprintf(w, "[%6s->%6s]", segment.Start.Truncate(time.Millisecond), segment.End.Truncate(time.Millisecond))
		fmt.Fprintln(w, " ", segment.Text)
	}
}

// Return srtTimestamp
func srtTimestamp(t time.Duration) string {
	return fmt.Sprintf("%02d:%02d:%02d,%03d", t/time.Hour, (t%time.Hour)/time.Minute, (t%time.Minute)/time.Second, (t%time.Second)/time.Millisecond)
}

// URLForModel returns the URL for the given model on huggingface.co
func URLForModel(model string) (string, error) {
	if filepath.Ext(model) != srcExt {
		model += srcExt
	}
	url, err := url.Parse(srcUrl)
	if err != nil {
		return "", err
	} else {
		url.Path = filepath.Join(url.Path, model)
	}
	return url.String(), nil
}

// Download downloads the model from the given URL to the given output directory
func Download(ctx context.Context, p io.Writer, model, out string) (string, error) {
	// Create HTTP client
	client := http.Client{
		// Timeout: 10 * time.Second,
	}

	// Initiate the download
	req, err := http.NewRequest("GET", model, nil)
	if err != nil {
		return "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%s: %s", model, resp.Status)
	}

	// If output file exists and is the same size as the model, skip
	path := filepath.Join(out, filepath.Base(model))
	if info, err := os.Stat(path); err == nil && info.Size() == resp.ContentLength {
		log.Println("Skipping", path, "as it already exists")
		return path, nil
	}

	// Create file
	w, err := os.Create(path)
	if err != nil {
		return "", err
	}
	defer w.Close()

	// Report
	log.Println("Downloading", model, "to", out)

	// Progressively download the model
	data := make([]byte, bufSize)
	count, pct := int64(0), int64(0)
	ticker := time.NewTicker(5 * time.Second)
	for {
		select {
		case <-ctx.Done():
			// Cancelled, return error
			return path, ctx.Err()
		case <-ticker.C:
			pct = DownloadReport(p, pct, count, resp.ContentLength)
		default:
			// Read body
			n, err := resp.Body.Read(data)
			if err != nil {
				DownloadReport(p, pct, count, resp.ContentLength)
				return path, err
			} else if m, err := w.Write(data[:n]); err != nil {
				return path, err
			} else {
				count += int64(m)
			}
		}
	}
}

// Report periodically reports the download progress when percentage changes
func DownloadReport(w io.Writer, pct, count, total int64) int64 {
	pct_ := count * 100 / total
	if pct_ > pct {
		fmt.Fprintf(w, "  ...%d MB written (%d%%)\n", count/1e6, pct_)
	}
	return pct_
}
