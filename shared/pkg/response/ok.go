package response

import (
	"encoding/json"
	"net/http"
)

func RespondOK(w http.ResponseWriter, data any) {
	raw, err := json.Marshal(data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)

		return
	}

	if data == nil {
		w.WriteHeader(http.StatusOK)

		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(raw)
}
