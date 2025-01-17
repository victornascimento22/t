package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
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

type ImageData struct {
	Data           []byte
	Index          int
	TransitionTime int
}

const (
	PORT           = "8081"
	TEMP_DIR       = "/tmp/slideshow"
	MIN_TRANSITION = 1
)

var (
	imageMap    = make(map[int]ImageData)
	imageMutex  sync.RWMutex
	displayChan = make(chan struct{}, 1)
)

func initTempDir() error {
	if err := os.RemoveAll(TEMP_DIR); err != nil {
		return fmt.Errorf("erro ao limpar diretório temporário: %w", err)
	}

	if err := os.MkdirAll(TEMP_DIR, 0755); err != nil {
		return fmt.Errorf("erro ao criar diretório temporário: %w", err)
	}

	return nil
}

func isValidImage(data []byte) bool {
	_, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		log.Printf("Erro ao verificar formato da imagem: %v", err)
		return false
	}
	log.Printf("Formato da imagem detectado: %s", format)
	return true
}

// Exibe a imagem usando feh com configuração melhorada
func showImage(imageData []byte, index int) error {
	if !isValidImage(imageData) {
		return fmt.Errorf("dados inválidos de imagem")
	}

	tempPath := filepath.Join(TEMP_DIR, fmt.Sprintf("screen_%d.png", index))

	if err := os.WriteFile(tempPath, imageData, 0644); err != nil {
		return fmt.Errorf("erro ao escrever imagem temporária: %w", err)
	}

	defer os.Remove(tempPath)

	log.Printf("Tentando exibir imagem %d de %s", index, tempPath)

	// Verificar se o arquivo existe
	if _, err := os.Stat(tempPath); err != nil {
		return fmt.Errorf("arquivo não encontrado: %v", err)
	}

	// Tentar diferentes configurações do DISPLAY
	displays := []string{":0", ":0.0"}
	var lastErr error

	for _, display := range displays {
		// Configurar ambiente
		env := append(os.Environ(),
			fmt.Sprintf("DISPLAY=%s", display),
			"XAUTHORITY=/home/"+os.Getenv("USER")+"/.Xauthority",
		)

		// Executar feh
		cmd := exec.Command("feh",
			"--fullscreen",
			"--bg-scale",
			"--hide-pointer",
			"--reload", "1",
			tempPath,
		)
		cmd.Env = env

		// Capturar saída padrão e de erro
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		log.Printf("Executando feh com DISPLAY=%s", display)
		err := cmd.Run()

		if err != nil {
			lastErr = fmt.Errorf("erro com DISPLAY=%s: %v\nstdout: %s\nstderr: %s",
				display, err, stdout.String(), stderr.String())
			log.Printf("Tentativa falhou: %v", lastErr)
			continue
		}

		log.Printf("Imagem exibida com sucesso usando DISPLAY=%s", display)
		return nil
	}

	return fmt.Errorf("todas as tentativas falharam: %v", lastErr)
}

func startSlideshow() {
	var lastIndex int
	for range displayChan {
		imageMutex.RLock()
		if len(imageMap) == 0 {
			imageMutex.RUnlock()
			time.Sleep(time.Second)
			displayChan <- struct{}{}
			continue
		}

		// Encontrar próxima imagem
		var nextImage ImageData
		found := false

		// Tentar próxima imagem na sequência
		for i := lastIndex + 1; i <= len(imageMap); i++ {
			if img, ok := imageMap[i]; ok {
				nextImage = img
				lastIndex = i
				found = true
				break
			}
		}

		// Se não encontrou, voltar ao início
		if !found {
			for i := 0; i <= lastIndex; i++ {
				if img, ok := imageMap[i]; ok {
					nextImage = img
					lastIndex = i
					found = true
					break
				}
			}
		}

		imageMutex.RUnlock()

		if !found {
			log.Printf("Nenhuma imagem encontrada para exibir")
			time.Sleep(time.Second)
			displayChan <- struct{}{}
			continue
		}

		if err := showImage(nextImage.Data, nextImage.Index); err != nil {
			log.Printf("Erro ao exibir imagem %d: %v", nextImage.Index, err)
			time.Sleep(time.Second)
		} else {
			transitionTime := nextImage.TransitionTime
			if transitionTime < MIN_TRANSITION {
				transitionTime = MIN_TRANSITION
			}
			log.Printf("Aguardando %d segundos antes da próxima imagem", transitionTime)
			time.Sleep(time.Duration(transitionTime) * time.Second)
		}

		displayChan <- struct{}{}
	}
}

func handleWebhook(w http.ResponseWriter, r *http.Request) {
	var payload ScreenPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "Erro ao ler payload", http.StatusBadRequest)
		return
	}

	imageBytes, err := base64.StdEncoding.DecodeString(payload.Image)
	if err != nil {
		http.Error(w, "Erro ao decodificar base64", http.StatusBadRequest)
		return
	}

	if !isValidImage(imageBytes) {
		http.Error(w, "Dados inválidos de imagem", http.StatusBadRequest)
		return
	}

	if payload.TransitionTime < MIN_TRANSITION {
		payload.TransitionTime = MIN_TRANSITION
	}

	imageMutex.Lock()
	imageMap[payload.Index] = ImageData{
		Data:           imageBytes,
		Index:          payload.Index,
		TransitionTime: payload.TransitionTime,
	}
	numImages := len(imageMap)
	imageMutex.Unlock()

	log.Printf("Recebida imagem %d com tempo de transição %ds. Total de imagens: %d",
		payload.Index, payload.TransitionTime, numImages)

	if numImages == 1 {
		log.Printf("Iniciando slideshow com a primeira imagem")
		displayChan <- struct{}{}
	}

	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "Imagem %d recebida com sucesso", payload.Index)
}

func main() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds | log.Lshortfile)

	if err := initTempDir(); err != nil {
		log.Fatalf("Erro ao inicializar diretório temporário: %v", err)
	}

	// Verificar se o feh está instalado
	if _, err := exec.LookPath("feh"); err != nil {
		log.Fatalf("feh não está instalado: %v", err)
	}

	go startSlideshow()

	http.HandleFunc("/webhook", handleWebhook)
	log.Printf("Servidor rodando em http://localhost:%s\n", PORT)
	log.Fatal(http.ListenAndServe(":"+PORT, nil))
}
