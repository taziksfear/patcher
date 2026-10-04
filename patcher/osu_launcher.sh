#!/bin/bash

# 1. Показываем нативное GTK окно
SERVER=$(zenity --list \
    --title="osu! Server Launcher" \
    --text="Select server:" \
    --radiolist \
    --column="Select" --column="Server" \
    TRUE "akatsuki.gg" \
    FALSE "gatari.pw" \
    FALSE "bancho" \
    --width=300 --height=250)

# 2. Если нажали отмену или закрыли окно - ничего не делаем
if [ -z "$SERVER" ]; then
    exit 0
fi

# 3. Сохраняем выбор в файл для нашего C# патчера
echo "$SERVER" > ~/.local/share/osu-wine/osu!/server.txt

# 4. Запускаем лаунчер osu-winello (он запустит наш патчер внутри контейнера)
~/.local/bin/osu-wine