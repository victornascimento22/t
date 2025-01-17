package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"sync"
)

type ScreenPayload struct {
	Image          string `json:"image"`
	Index          int    `json:"index"`
	TransitionTime int    `json:"transition_time"`
}

const PORT = "8081"

var (
	fehCmd       *exec.Cmd
	fehMutex     sync.Mutex
	currentIndex int
)

func showImage(imageData []byte) {
	fehMutex.Lock()
	defer fehMutex.Unlock()

	if fehCmd != nil && fehCmd.Process != nil {
		fehCmd.Process.Kill()
		fehCmd.Wait()
	}

	pr, pw := io.Pipe()

	fehCmd = exec.Command("feh",
		"-F",
		"--hide-pointer",
		"--force-aliasing",
		"--zoom", "fill",
		"--high-quality",
		"--scale-down",
		"--conversion-timeout", "10",
		"--draw-tinted",
		"-d",
		"-",
		"no-info",
	)

	fehCmd.Stdin = pr
	fehCmd.Env = append(os.Environ(), "DISPLAY=:0")

	var stderr bytes.Buffer
	fehCmd.Stderr = &stderr

	if err := fehCmd.Start(); err != nil {
		log.Printf("Erro ao iniciar feh: %v\nErro: %s", err, stderr.String())
		return
	}

	go func() {
		_, err := pw.Write(imageData)
		if err != nil {
			log.Printf("Erro ao escrever imagem no pipe: %v", err)
		}
		pw.Close()
	}()

	log.Printf("Exibindo screenshot (indice: %d)", currentIndex)
}

func handleWebhook(w http.ResponseWriter, r *http.Request) {
	log.Printf("Recebendo screenshot de %s", r.RemoteAddr)

	var payload ScreenPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		log.Printf("Erro ao decodificar JSON: %v", err)
		http.Error(w, "Erro ao ler payload", http.StatusBadRequest)
		return
	}

	log.Printf("Payload recebido: indice=%d, transicao=%ds",
		payload.Index,
		payload.TransitionTime,
	)

	imageBytes, err := base64.StdEncoding.DecodeString(payload.Image)
	if err != nil {
		log.Printf("Erro ao decodificar imagem: %v", err)
		http.Error(w, "Erro ao decodificar imagem", http.StatusBadRequest)
		return
	}

	currentIndex = payload.Index
	showImage(imageBytes)

	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "Screenshot %d recebida e exibida com sucesso", payload.Index)
}

func main() {
	http.HandleFunc("/webhook", handleWebhook)
	log.Printf("Servidor rodando em http://localhost:%s\n", PORT)
	log.Fatal(http.ListenAndServe(":"+PORT, nil))
}
