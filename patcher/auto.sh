#!/bin/bash

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )"

OSU_DIR="$HOME/.local/share/osu-wine/osu!"
REAL_OSU="$OSU_DIR/real_osu"

if [ ! -f "$SCRIPT_DIR/osu!.exe" ]; then
    echo "положи auto.sh и новый osu!.exe в одну папку и запусти снова. | put auto.sh and my osu!.exe in same folder and try again. "
    exit 1
fi

if [ ! -d "$OSU_DIR" ]; then
    echo "сорян, но если у тебя нестандартный путь, придется делать всё ручками по гайду))))) | sorry, but your osu has no default dir, do everything yourself as i said in readme.md"
    exit 1
fi

mkdir -p "$REAL_OSU"

cd "$OSU_DIR" || exit
shopt -s extglob dotglob
mv !(real_osu) "$REAL_OSU/" 2>/dev/null
shopt -u extglob dotglob
cp "$SCRIPT_DIR/osu!.exe" "$OSU_DIR/osu!.exe"

LAUNCHER_PATH="$SCRIPT_DIR/osu_launcher.sh"

cat << 'EOF' > "$LAUNCHER_PATH"
#!/bin/bash

OSU_DIR="$HOME/.local/share/osu-wine/osu!"
CUSTOM_SERVERS_FILE="$OSU_DIR/custom_servers.txt"

while true; do
    SERVERS=(TRUE "akatsuki.gg" FALSE "bancho")

    # Подтягиваем кастомные сервера юзера, если они есть
    if [ -f "$CUSTOM_SERVERS_FILE" ]; then
        while IFS= read -r line; do
            if [ -n "$line" ]; then
                SERVERS+=(FALSE "$line")
            fi
        done < "$CUSTOM_SERVERS_FILE"
    fi

    # Добавляем кнопку создания нового сервера в самый конец
    SERVERS+=(FALSE "[+] Add custom server...")

    SERVER=$(zenity --list \
        --title="osu! Server Launcher" \
        --text="Select server to play on:" \
        --radiolist \
        --column="Select" --column="Server" \
        "${SERVERS[@]}" \
        --width=300 --height=350)

    if [ -z "$SERVER" ]; then
        exit 0
    fi

    if [ "$SERVER" == "[+] Add custom server..." ]; then
        NEW_SERVER=$(zenity --entry \
            --title="Add Server" \
            --text="Enter new server domain (e.g. mino.cat):")

        if [ -n "$NEW_SERVER" ]; then
            echo "$NEW_SERVER" >> "$CUSTOM_SERVERS_FILE"
        fi
    else
        break
    fi
done

echo "$SERVER" > "$OSU_DIR/server.txt"
~/.local/bin/osu-wine
EOF

chmod +x "$LAUNCHER_PATH"

echo "Я КОНЧИЛ | done"