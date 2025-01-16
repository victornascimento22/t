package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"time"
)

type Payload struct {
	Images         []string `json:"images"`
	TransitionTime int      `json:"transitionTime"`
}

var (
	imageList    []string
	imageMutex   sync.Mutex
	currentIndex int
)

func showImage(imagePath string) error {
	cmd := exec.Command("feh", "--fullscreen", imagePath)
	return cmd.Run()
}

func startSlideshow(transitionTime int) {
	for {
		imageMutex.Lock()
		if len(imageList) == 0 {
			imageMutex.Unlock()
			time.Sleep(1 * time.Second)
			continue
		}

		currentImage := imageList[currentIndex]
		log.Printf("Exibindo imagem %d: %s", currentIndex, currentImage)

		if err := showImage(currentImage); err != nil {
			log.Printf("Erro ao exibir imagem %s: %v", currentImage, err)
		}

		currentIndex = (currentIndex + 1) % len(imageList)
		imageMutex.Unlock()

		time.Sleep(time.Duration(transitionTime) * time.Second)
	}
}

func webhookHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Método não permitido", http.StatusMethodNotAllowed)
		return
	}

	var payload Payload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "Erro ao decodificar payload", http.StatusBadRequest)
		log.Printf("Erro ao decodificar payload: %v", err)
		return
	}

	imageMutex.Lock()
	imageList = append(imageList, payload.Images...)
	if currentIndex >= len(imageList) {
		currentIndex = 0
	}
	imageMutex.Unlock()

	log.Printf("Novas imagens adicionadas: %v", payload.Images)
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Imagens adicionadas com sucesso!"))
}

func main() {
	transitionTime := 5 // Default transition time in seconds
	if len(os.Args) > 1 {
		if customTime, err := strconv.Atoi(os.Args[1]); err == nil {
			transitionTime = customTime
		}
	}

	go startSlideshow(transitionTime)

	http.HandleFunc("/webhook", webhookHandler)
	log.Println("Servidor iniciado na porta 8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatalf("Erro ao iniciar servidor: %v", err)
	}
}
