package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

type ScreenPayload struct {
	Image          string `json:"image"`
	Index          int    `json:"index"`
	TransitionTime int    `json:"transition_time"`
}

const (
	PORT        = "8081"
	ImageFolder = "/tmp/screenshots"
)

var (
	imageList    []string
	imageMutex   sync.Mutex
	currentIndex int
	isRunning    bool
)

func cleanImageFolder() {
	files, err := filepath.Glob(filepath.Join(ImageFolder, "screen_*.png"))
	if err != nil {
		log.Printf("Erro ao listar imagens: %v", err)
		return
	}

	for _, file := range files {
		if err := os.Remove(file); err != nil {
			log.Printf("Erro ao remover arquivo %s: %v", file, err)
		}
	}

	imageList = []string{} // Limpa a lista de imagens
	currentIndex = 0
}

func saveImage(index int, imageData []byte) (string, error) {
	imagePath := filepath.Join(ImageFolder, fmt.Sprintf("screen_%d.png", index))

	if err := os.WriteFile(imagePath, imageData, 0644); err != nil {
		return "", fmt.Errorf("erro ao salvar imagem: %v", err)
	}

	imageMutex.Lock()
	defer imageMutex.Unlock()

	// Verifica se a imagem ja existe na lista
	for _, img := range imageList {
		if img == imagePath {
			return imagePath, nil
		}
	}

	// Adiciona nova imagem a lista
	imageList = append(imageList, imagePath)
	log.Printf("Nova imagem adicionada: %s. Total: %d", imagePath, len(imageList))

	return imagePath, nil
}

func showImage(imagePath string) error {
	cmd := exec.Command("feh",
		"-F",               // Tela cheia
		"--hide-pointer",   // Esconde cursor
		"--force-aliasing", // Melhor qualidade
		"--zoom", "fill",   // Preenche tela
		"--high-quality", // Alta qualidade
		"--scale-down",   // Ajusta escala
		"--borderless",   // Sem bordas
		"--no-menus",     // Sem menus
		"--quiet",        // Sem logs
		imagePath,
	)

	cmd.Env = append(os.Environ(), "DISPLAY=:0")
	return cmd.Run()
}

func startSlideshow() {
	if isRunning {
		return
	}

	isRunning = true

	go func() {
		for {
			imageMutex.Lock()
			if len(imageList) == 0 {
				imageMutex.Unlock()
				time.Sleep(time.Second)
				continue
			}

			// Pega imagem atual
			currentImage := imageList[currentIndex]
			imageMutex.Unlock()

			// Exibe imagem
			if err := showImage(currentImage); err != nil {
				log.Printf("Erro ao exibir imagem %s: %v", currentImage, err)
				time.Sleep(time.Second)
				continue
			}

			imageMutex.Lock()
			// Avanca para proxima imagem
			currentIndex = (currentIndex + 1) % len(imageList)
			imageMutex.Unlock()

			// Aguarda antes da proxima imagem
			time.Sleep(5 * time.Second)
		}
	}()
}

func handleWebhook(w http.ResponseWriter, r *http.Request) {
	var payload ScreenPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "Erro ao ler payload", http.StatusBadRequest)
		return
	}

	imageBytes, err := base64.StdEncoding.DecodeString(payload.Image)
	if err != nil {
		http.Error(w, "Erro ao decodificar imagem", http.StatusBadRequest)
		return
	}

	imagePath, err := saveImage(payload.Index, imageBytes)
	if err != nil {
		http.Error(w, "Erro ao salvar imagem", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "Imagem %d salva: %s", payload.Index, imagePath)
}

func main() {
	if err := os.MkdirAll(ImageFolder, 0755); err != nil {
		log.Fatalf("Erro ao criar diretorio: %v", err)
	}

	cleanImageFolder()
	startSlideshow()

	http.HandleFunc("/webhook", handleWebhook)
	log.Printf("Servidor rodando na porta %s", PORT)
	log.Fatal(http.ListenAndServe(":"+PORT, nil))
}
