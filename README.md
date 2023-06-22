# Speech-To-Text
Whispering into the Future: Reinventing Speech-to-Text Transcriptions with Go and Whisper

# YTrans

YTrans is a command-line tool to transcribe YouTube videos using the [whisper](https://github.com/ggerganov/whisper.cpp) library.

## Prerequisites

- Docker
- A valid YouTube link for testing

## Getting started

1. Clone the repo:

   ```
   git clone https://github.com/your-repo/ytrans.git
   cd ytrans
   ```

2. Create a docker network

`docker network create surrealdb-network`

3. Run SurrealDB:

```bash
sh surrealdb.sh
```

4. Build the Docker image:

   ```bash
   sh buildx.sh
   ```

5. Run the Docker container:

   ```bash
   sh runx.sh
   ```

6. Now you can use the `ytrans` tool to transcribe YouTube videos.

## Usage

To transcribe a single YouTube video, use the following command format:

```
ytrans <video-link>
```

Replace `<video-link>` with the link to the YouTube video you want to transcribe.

Example:

```
ytrans https://www.youtube.com/watch?v=example
```

This command will download the audio from the specified YouTube video, convert it to WAV format, and transcribe the audio using the whisper library.

## License

This project is licensed under the MIT License. See [LICENSE](LICENSE) for more information.
