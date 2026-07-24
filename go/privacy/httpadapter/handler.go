package httpadapter

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/yueli-official/foundation/go/privacy"
)

type HandlerOptions struct {
	Host     privacy.OwnerHost
	MaxBytes int64
}

func NewHandler(options HandlerOptions) (http.Handler, error) {
	if options.Host == nil {
		return nil, errors.New("privacy/httpadapter: host is required")
	}
	maxBytes := options.MaxBytes
	if maxBytes == 0 {
		maxBytes = 256 << 10
	}
	if maxBytes < 4096 || maxBytes > 4<<20 {
		return nil, errors.New("privacy/httpadapter: max bytes is out of range")
	}
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			writer.Header().Set("Allow", http.MethodPost)
			http.Error(writer, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if contentType := request.Header.Get("Content-Type"); contentType != "application/json" {
			http.Error(writer, "application/json required", http.StatusUnsupportedMediaType)
			return
		}
		reader := http.MaxBytesReader(writer, request.Body, maxBytes)
		decoder := json.NewDecoder(reader)
		decoder.DisallowUnknownFields()
		var command privacy.OwnerCommand
		if err := decoder.Decode(&command); err != nil {
			http.Error(writer, "invalid owner command", http.StatusBadRequest)
			return
		}
		if err := ensureJSONEOF(decoder); err != nil {
			http.Error(writer, "invalid owner command", http.StatusBadRequest)
			return
		}
		receipt, err := options.Host.Handle(request.Context(), command)
		if err != nil {
			status := http.StatusUnprocessableEntity
			var typed *privacy.Error
			if errors.As(err, &typed) && typed.Retryable {
				status = http.StatusServiceUnavailable
			}
			http.Error(writer, "owner command failed", status)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		writer.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(writer).Encode(receipt)
	}), nil
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var extra any
	err := decoder.Decode(&extra)
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err == nil {
		return errors.New("trailing JSON value")
	}
	return err
}
