#!/bin/bash

# Instala dependências necessárias
sudo apt-get update
sudo apt-get install -y feh  # feh é um visualizador de imagens leve

# Inicia o feh em modo tela cheia
while true; do
    feh --full-screen --auto-zoom /home/pi/display/current.png
    sleep 1
done 