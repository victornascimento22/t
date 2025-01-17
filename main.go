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
	imagePaths       []string
	imageDir         = "./images"
)

// initFeh inicia o feh em modo slideshow com as imagens salvas
func initFeh(transitionTime int) error {
	fehMutex.Lock()
	defer fehMutex.Unlock()

	// Mata qualquer instância existente do feh
	exec.Command("pkill", "feh").Run()

	// Inicia o feh com as imagens salvas
	args := []string{
		"-R", "1", // Recarrega a cada 1 segundo
		"-F",                                    // Modo tela cheia
		"-Z",                                    // Zoom automático
		"-D", fmt.Sprintf("%d", transitionTime), // Delay baseado no tempo enviado
		"-Y",     // Esconde o cursor do mouse
		imageDir, // Diretório das imagens
	}
	fehCmd = exec.Command("feh", args...)

	if err := fehCmd.Start(); err != nil {
		return fmt.Errorf("erro ao iniciar feh: %v", err)
	}

	return nil
}

// cleanUpImages remove todas as imagens salvas
func cleanUpImages() error {
	files, err := ioutil.ReadDir(imageDir)
	if err != nil {
		return fmt.Errorf("erro ao ler diretório de imagens: %v", err)
	}
	for _, file := range files {
		if err := os.Remove(fmt.Sprintf("%s/%s", imageDir, file.Name())); err != nil {
			return fmt.Errorf("erro ao remover arquivo %s: %v", file.Name(), err)
		}
	}
	imagePaths = nil
	return nil
}

// adjustImage ajusta a imagem recebida (trim, redimensionamento, etc.)
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

// handleWebhook lida com o webhook recebido, ajustando e salvando as imagens
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

	outputPath := fmt.Sprintf("%s/output-%d.png", imageDir, payload.Index)
	if err := ioutil.WriteFile(outputPath, adjustedImage, 0644); err != nil {
		log.Printf("Erro ao salvar imagem ajustada: %v", err)
		http.Error(w, "Erro ao salvar imagem ajustada", http.StatusInternalServerError)
		return
	}

	imagePaths = append(imagePaths, outputPath)
	log.Printf("Imagem ajustada salva em: %s", outputPath)

	// Atualiza o slideshow do feh com o novo tempo de transição
	if err := initFeh(payload.TransitionTime); err != nil {
		log.Printf("Erro ao reiniciar feh: %v", err)
	}

	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "Screenshot %d recebida, ajustada e salva com sucesso", payload.Index)
}

func main() {
	// Cria o diretório de imagens, se não existir
	if err := os.MkdirAll(imageDir, 0755); err != nil {
		log.Fatalf("Erro ao criar diretório de imagens: %v", err)
	}

	// Limpa imagens antigas ao iniciar
	if err := cleanUpImages(); err != nil {
		log.Fatalf("Erro ao limpar imagens antigas: %v", err)
	}

	// Inicia o feh antes de começar o servidor
	if err := initFeh(5); err != nil {
		log.Fatal(err)
	}

	// Configura um handler para finalizar o feh e limpar imagens ao fechar
	defer func() {
		fehMutex.Lock()
		if fehCmd != nil {
			fehCmd.Process.Kill()
		}
		fehMutex.Unlock()
		if err := cleanUpImages(); err != nil {
			log.Printf("Erro ao limpar imagens na saída: %v", err)
		}
	}()

	http.HandleFunc("/webhook", handleWebhook)
	log.Printf("Servidor rodando em http://localhost:%s\n", PORT)
	log.Fatal(http.ListenAndServe(":"+PORT, nil))
}
