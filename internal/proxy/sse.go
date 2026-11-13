package proxy

import (
	"bufio"
	"bytes"
	"io"
	"log"
	"net/http"

	"github.com/bigduu/bodhi-server/internal/converter"
)

func streamNative(w http.ResponseWriter, resp *http.Response, flusher http.Flusher, protocol inferenceProtocol) (int, int) {
	var input, output int
	anthropic := converter.NewAnthropicStreamConverter()
	gemini := converter.NewGeminiStreamConverter()
	// Limit only metering work. Fragments are relayed even for large events.
	const maxMeteredEventBytes = 1024 * 1024
	reader := bufio.NewReaderSize(resp.Body, 32*1024)
	var data bytes.Buffer
	incompleteLine := false
	skipEvent := false
	for {
		line, err := reader.ReadSlice('\n')
		switch {
		case err == bufio.ErrBufferFull:
			incompleteLine = true
			skipEvent = true
		case !incompleteLine:
			field := bytes.TrimRight(line, "\r\n")
			if len(line) > 0 && len(field) == 0 && err == nil {
				if !skipEvent && data.Len() > 0 {
					switch protocol {
					case anthropicMessages:
						_, _ = anthropic.Convert("", data.String())
						input, output = anthropic.Usage()
					case geminiContent:
						_, _ = gemini.Convert("", data.String())
						input, output = gemini.Usage()
					case openAIResponses:
						if in, out, ok := converter.ExtractResponsesUsage(data.Bytes()); ok {
							input, output = in, out
						}
					}
				}
				skipEvent = false
				data.Reset()
			} else if value, ok := bytes.CutPrefix(field, []byte("data:")); ok && !skipEvent {
				value = bytes.TrimPrefix(value, []byte(" "))
				if data.Len()+len(value)+1 <= maxMeteredEventBytes {
					data.Write(value)
					data.WriteByte('\n')
				} else {
					skipEvent = true
				}
			}
		default:
			incompleteLine = false
		}
		if len(line) > 0 {
			if _, writeErr := w.Write(line); writeErr != nil {
				return input, output
			}
			flusher.Flush()
		}
		if err != nil && err != bufio.ErrBufferFull {
			if err != io.EOF {
				log.Printf("stream error: %v", err)
			}
			return input, output
		}
	}
}

func StreamSSE(w http.ResponseWriter, resp *http.Response) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	copyProviderResponseHeaders(w.Header(), resp.Header, false)
	w.WriteHeader(resp.StatusCode)
	flusher.Flush()

	buf := make([]byte, 8192)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			w.Write(buf[:n])
			flusher.Flush()
		}
		if err != nil {
			if err != io.EOF {
				log.Printf("stream error: %v", err)
			}
			break
		}
	}
}

func StandardProxy(w http.ResponseWriter, resp *http.Response) {
	copyProviderResponseHeaders(w.Header(), resp.Header, false)

	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}
