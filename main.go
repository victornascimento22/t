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
	Images         []string `json:"images"`          // Lista de URLs base64
	TransitionTime int      `json:"transition_time"` // Tempo de transição entre imagens
}

const (
	PORT        = "8081"
	ImageFolder = "/tmp/screenshots"
)

var (
	images       []string // Lista de imagens salvas
	imageMutex   sync.Mutex
	currentIndex int
	transition   int = 3 // Tempo de transição padrão em segundos
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

// Salva as novas imagens no diretório e atualiza a lista
func saveImages(imageDataList []string) error {
	imageMutex.Lock()
	defer imageMutex.Unlock()

	// Limpa imagens existentes
	cleanImageFolder()
	images = nil

	// Salva cada imagem individualmente
	for i, imageData := range imageDataList {
		imageBytes, err := base64.StdEncoding.DecodeString(imageData)
		if err != nil {
			log.Printf("Erro ao decodificar imagem %d: %v", i, err)
			return fmt.Errorf("erro ao decodificar imagem: %w", err)
		}

		imagePath := filepath.Join(ImageFolder, fmt.Sprintf("screen_%d.png", i))
		err = os.WriteFile(imagePath, imageBytes, 0644)
		if err != nil {
			log.Printf("Erro ao salvar imagem %d: %v", i, err)
			return fmt.Errorf("erro ao salvar imagem: %w", err)
		}

		// Adiciona à lista de imagens
		images = append(images, imagePath)
		log.Printf("Imagem salva: %s", imagePath)
	}

	return nil
}

// Loop contínuo para exibir imagens em sequência
func startSlideshow() {
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
		time.Sleep(time.Duration(transition) * time.Second)
	}
}

// Exibe a imagem com o comando feh em tela cheia
func showImage(imagePath string) {
	log.Printf("Exibindo imagem: %s", imagePath)

	cmd := []string{
		"feh",
		"--fullscreen",   // Ativa o modo tela cheia
		"--hide-pointer", // Esconde o cursor do mouse
		"--force-aliasing",
		"--zoom", "fill",
		"--high-quality",
		"--scale-down",
		"--no-info", // Não exibe informações na tela
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
	log.Printf("Recebendo payload de %s", r.RemoteAddr)

	var payload ScreenPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		log.Printf("Erro ao decodificar JSON: %v", err)
		http.Error(w, "Erro ao ler payload", http.StatusBadRequest)
		return
	}

	log.Printf("Payload recebido: imagens=%d, transicao=%ds",
		len(payload.Images),
		payload.TransitionTime,
	)

	// Atualiza o tempo de transição se fornecido
	if payload.TransitionTime > 0 {
		transition = payload.TransitionTime
	}

	// Salva as imagens no diretório
	err := saveImages(payload.Images)
	if err != nil {
		log.Printf("Erro ao salvar imagens: %v", err)
		http.Error(w, "Erro ao salvar imagens", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "Imagens recebidas e salvas com sucesso")
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
	go startSlideshow()

	http.HandleFunc("/webhook", handleWebhook)
	log.Printf("Servidor rodando em http://localhost:%s\n", PORT)
	log.Fatal(http.ListenAndServe(":"+PORT, nil))
}
