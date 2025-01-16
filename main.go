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
	images       []string // Lista de imagens
	imageMutex   sync.Mutex
	currentIndex int
)

// Limpa o diretório de imagens antigas
func cleanImageFolder() {
	files, err := filepath.Glob(filepath.Join(ImageFolder, "screen_*.png"))
	if err != nil {
		log.Printf("Erro ao listar imagens no diretório: %v", err)
		return
	}

	for _, file := range files {
		err := os.Remove(file)
		if err != nil {
			log.Printf("Erro ao remover imagem %s: %v", file, err)
		} else {
			log.Printf("Imagem removida: %s", file)
		}
	}
}

// Salva a nova imagem no diretório e atualiza a lista
func saveImage(index int, imageData []byte) (string, error) {
	imagePath := filepath.Join(ImageFolder, fmt.Sprintf("screen_%d.png", index))
	err := os.WriteFile(imagePath, imageData, 0644)
	if err != nil {
		return "", fmt.Errorf("erro ao salvar imagem: %w", err)
	}

	// Atualiza a lista de imagens
	imageMutex.Lock()
	images = append(images, imagePath)
	imageMutex.Unlock()

	return imagePath, nil
}

// Loop contínuo para exibir imagens em sequência
func startSlideshow(transitionTime int) {
	for {
		imageMutex.Lock()
		if len(images) == 0 {
			imageMutex.Unlock()
			time.Sleep(1 * time.Second) // Aguarda se não houver imagens
			continue
		}

		// Seleciona a imagem atual
		image := images[currentIndex]
		imageMutex.Unlock()

		// Exibe a imagem
		showImage(image)

		// Atualiza o índice para a próxima imagem
		imageMutex.Lock()
		currentIndex = (currentIndex + 1) % len(images)
		imageMutex.Unlock()

		// Aguarda o tempo de transição
		time.Sleep(time.Duration(transitionTime) * time.Second)
	}
}

// Exibe a imagem com o comando feh
func showImage(imagePath string) {
	log.Printf("Exibindo imagem: %s", imagePath)

	cmd := []string{
		"feh",
		"--hide-pointer",
		"--force-aliasing",
		"--zoom", "fill",
		"--high-quality",
		"--scale-down",
		"--no-info",
		imagePath,
	}

	err := executeCommand("feh", cmd...)
	if err != nil {
		log.Printf("Erro ao exibir imagem %s: %v", imagePath, err)
	}
}

// Executa comandos do sistema
func executeCommand(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Env = append(os.Environ(), "DISPLAY=:0")

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("erro ao executar comando %s: %w", name, err)
	}
	return nil
}

// Manipula as solicitações recebidas no webhook
func handleWebhook(w http.ResponseWriter, r *http.Request) {
	log.Printf("Recebendo imagem de %s", r.RemoteAddr)

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

	// Salva a imagem no diretório
	_, err = saveImage(payload.Index, imageBytes)
	if err != nil {
		log.Printf("Erro ao salvar imagem: %v", err)
		http.Error(w, "Erro ao salvar imagem", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "Imagem %d recebida e salva com sucesso", payload.Index)
}

func main() {
	// Limpa o diretório ao iniciar o programa
	cleanImageFolder()

	// Garante que o diretório existe
	if _, err := os.Stat(ImageFolder); os.IsNotExist(err) {
		err := os.MkdirAll(ImageFolder, 0755)
		if err != nil {
			log.Fatalf("Erro ao criar diretório %s: %v", ImageFolder, err)
		}
	}

	// Inicia o slideshow em um goroutine separado
	go startSlideshow(5) // Tempo de transição padrão: 3 segundos

	http.HandleFunc("/webhook", handleWebhook)
	log.Printf("Servidor rodando em http://localhost:%s\n", PORT)
	log.Fatal(http.ListenAndServe(":"+PORT, nil))
}
