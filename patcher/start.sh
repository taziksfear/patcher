#!/bin/bash

OSU_DIR="$HOME/.local/share/osu-wine/osu!"
CUSTOM_SERVERS_FILE="$OSU_DIR/custom_servers.txt"

while true; do
    SERVERS=(TRUE "akatsuki.gg" FALSE "gatari.pw" FALSE "bancho" FALSE "ppy.sh" FALSE "osu.ppy.sh")

    if [ -f "$CUSTOM_SERVERS_FILE" ]; then
        while IFS= read -r line; do
            if [ -n "$line" ]; then
                SERVERS+=(FALSE "$line")
            fi
        done < "$CUSTOM_SERVERS_FILE"
    fi

    SERVERS+=(FALSE "add custom server...")

    SERVER=$(zenity --list \
        --title="osu! server launcher" \
        --text="select server to play on:" \
        --radiolist \
        --column="Select" --column="Server" \
        "${SERVERS[@]}" \
        --width=300 --height=350)

    if [ -z "$SERVER" ]; then
        exit 0
    fi

    if [ "$SERVER" == "add custom server" ]; then
        NEW_SERVER=$(zenity --entry \
            --title="add Server" \
            --text="enter new server domain")
        
        if [ -n "$NEW_SERVER" ]; then
            echo "$NEW_SERVER" >> "$CUSTOM_SERVERS_FILE"
        fi
    else
        break
    fi
done
CUSTOM_CLIENT_DIR="$OSU_DIR/clients/$SERVER"

if [ -d "$CUSTOM_CLIENT_DIR" ]; then
    echo "64 bit patcher for$SERVER"
    cd "$CUSTOM_CLIENT_DIR"
    EXE_FILE=$(ls *.exe | head -n 1)
    wine "$EXE_FILE"
else
    echo "32 bit patcher for $SERVER"
    echo "$SERVER" > "$OSU_DIR/server.txt"
    ~/.local/bin/osu-wine
fi