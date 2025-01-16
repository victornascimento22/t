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
	Image          string `json:"image"`           // Imagem em base64
	Index          int    `json:"index"`           // Índice da imagem
	TransitionTime int    `json:"transition_time"` // Tempo de transição em segundos
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

	// Mata processo anterior
	if fehCmd != nil && fehCmd.Process != nil {
		fehCmd.Process.Kill()
		fehCmd.Wait()
	}

	// Cria um pipe para passar a imagem para o feh
	pr, pw := io.Pipe()

	fehCmd = exec.Command("feh",
		"-F",               // Tela cheia
		"--hide-pointer",   // Esconde o cursor
		"--auto-zoom",      // Ajusta zoom automaticamente
		"--force-aliasing", // Força melhor qualidade
		"--quiet",          // Reduz logs
		"-",                // Lê da entrada padrão
	)

	fehCmd.Stdin = pr
	fehCmd.Env = append(os.Environ(), "DISPLAY=:0")

	var stderr bytes.Buffer
	fehCmd.Stderr = &stderr

	if err := fehCmd.Start(); err != nil {
		log.Printf("❌ Erro ao iniciar feh: %v\nErro: %s", err, stderr.String())
		return
	}

	// Escreve a imagem no pipe
	go func() {
		_, err := pw.Write(imageData)
		if err != nil {
			log.Printf("❌ Erro ao escrever imagem no pipe: %v", err)
		}
		pw.Close()
	}()

	log.Printf("✨ Exibindo screenshot (índice: %d)", currentIndex)
}

func handleWebhook(w http.ResponseWriter, r *http.Request) {
	log.Printf("📥 Recebendo screenshot de %s", r.RemoteAddr)

	var payload ScreenPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		log.Printf("❌ Erro ao decodificar JSON: %v", err)
		http.Error(w, "Erro ao ler payload", http.StatusBadRequest)
		return
	}

	log.Printf("📦 Payload recebido: índice=%d, transição=%ds",
		payload.Index,
		payload.TransitionTime,
	)

	// Decodifica a imagem de base64
	imageBytes, err := base64.StdEncoding.DecodeString(payload.Image)
	if err != nil {
		log.Printf("❌ Erro ao decodificar imagem: %v", err)
		http.Error(w, "Erro ao decodificar imagem", http.StatusBadRequest)
		return
	}

	// Atualiza o índice atual
	currentIndex = payload.Index

	// Exibe a imagem
	showImage(imageBytes)

	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "Screenshot %d recebida e exibida com sucesso", payload.Index)
}

func main() {
	http.HandleFunc("/webhook", handleWebhook)
	log.Printf("🚀 Servidor rodando em http://localhost:%s\n", PORT)
	log.Fatal(http.ListenAndServe(":"+PORT, nil))
}
