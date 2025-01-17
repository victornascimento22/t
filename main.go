package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
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
	imagemagickMutex sync.Mutex
	fehCmd           *exec.Cmd
	fehMutex         sync.Mutex
)

// initFeh inicia o feh em modo slideshow
func initFeh() error {
	fehMutex.Lock()
	defer fehMutex.Unlock()

	// Mata qualquer instância existente do feh
	if err := exec.Command("pkill", "feh").Run(); err != nil {
		log.Printf("Erro ao matar instância do feh: %v", err)
	}

	// Inicia o feh em modo slideshow
	fehCmd = exec.Command("feh",
		"-R", "1", // Recarrega a cada 1 segundo
		"-F",      // Modo tela cheia
		"-Z",      // Zoom automático
		"-D", "5", // Delay padrão de 5 segundos
		"-Y", // Esconde o cursor do mouse
		"./", // Diretório atual
	)

	if err := fehCmd.Start(); err != nil {
		return fmt.Errorf("erro ao iniciar feh: %v", err)
	}

	return nil
}

func adjustImage(imageData []byte) ([]byte, error) {
	imagemagickMutex.Lock()
	defer imagemagickMutex.Unlock()

	inputFile, err := ioutil.TempFile("", "input-*.png")
	if err != nil {
		return nil, fmt.Errorf("erro ao criar arquivo temporário de entrada: %v", err)
	}
	defer os.Remove(inputFile.Name())

	outputFile, err := ioutil.TempFile("", "output-*.png")
	if err != nil {
		return nil, fmt.Errorf("erro ao criar arquivo temporário de saída: %v", err)
	}
	defer os.Remove(outputFile.Name())

	if _, err := inputFile.Write(imageData); err != nil {
		return nil, fmt.Errorf("erro ao escrever imagem no arquivo de entrada: %v", err)
	}
	inputFile.Close()

	// Comando atualizado para remover bordas brancas e ajustar a imagem
	cmd := exec.Command("convert",
		inputFile.Name(),
		"-trim",                // Remove as bordas brancas
		"+repage",              // Redefine as coordenadas da imagem após o trim
		"-background", "black", // Define fundo preto
		"-gravity", "center", // Centraliza a imagem
		"-resize", "1920x1080^", // Redimensiona cobrindo a área mínima
		"-extent", "1920x1080", // Força o tamanho exato
		"-flatten",          // Achata todas as camadas
		"-compress", "JPEG", // Usa compressão JPEG
		"-quality", "100", // Máxima qualidade
		outputFile.Name(),
	)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("erro ao executar ImageMagick: %v\n%s", err, stderr.String())
	}

	adjustedImage, err := ioutil.ReadFile(outputFile.Name())
	if err != nil {
		return nil, fmt.Errorf("erro ao ler imagem ajustada: %v", err)
	}

	return adjustedImage, nil
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

	adjustedImage, err := adjustImage(imageBytes)
	if err != nil {
		log.Printf("Erro ao ajustar imagem: %v", err)
		http.Error(w, "Erro ao ajustar imagem", http.StatusInternalServerError)
		return
	}

	outputPath := fmt.Sprintf("output-%d.png", payload.Index)
	if err := ioutil.WriteFile(outputPath, adjustedImage, 0644); err != nil {
		log.Printf("Erro ao salvar imagem ajustada: %v", err)
		http.Error(w, "Erro ao salvar imagem ajustada", http.StatusInternalServerError)
		return
	}

	log.Printf("Imagem ajustada salva em: %s", outputPath)
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "Screenshot %d recebida, ajustada e salva com sucesso", payload.Index)
}

func statusHandler(w http.ResponseWriter, r *http.Request) {
	// Verifica se a aplicação está rodando (retorna true para online)
	// Retorne false para offline se a aplicação não estiver rodando
	isOnline := true // Ou false, dependendo do status da aplicação
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"online": isOnline})
}

func main() {
	// Inicia o feh antes de começar o servidor
	if err := initFeh(); err != nil {
		log.Fatal(err)
	}

	// Define o handler para o webhook e status
	http.HandleFunc("/webhook", handleWebhook)
	http.HandleFunc("/status", statusHandler)

	log.Printf("Servidor rodando em http://localhost:%s\n", PORT)
	log.Fatal(http.ListenAndServe(":"+PORT, nil))
}
