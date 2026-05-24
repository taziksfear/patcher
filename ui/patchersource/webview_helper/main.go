//go:build ignore

package main

/*
#cgo pkg-config: gtk+-3.0 webkit2gtk-4.1
#include <gtk/gtk.h>
#include <webkit2/webkit2.h>
#include <cairo.h>
#include <stdlib.h>
#include <stdio.h>

void cb_load(WebKitWebView *wv, WebKitLoadEvent ev, gpointer path) {
    if (ev != WEBKIT_LOAD_FINISHED && ev != WEBKIT_LOAD_COMMITTED) return;

    // Делаем скриншот через 800мс после загрузки
    // (используем простой подход — сохраняем surface)
    GdkWindow *win = gtk_widget_get_window(gtk_widget_get_toplevel(GTK_WIDGET(wv)));
    if (!win) return;

    int w = gdk_window_get_width(win);
    int h = gdk_window_get_height(win);
    if (w <= 0 || h <= 0) return;

    cairo_surface_t *surf = cairo_image_surface_create(CAIRO_FORMAT_ARGB32, w, h);
    cairo_t *cr = cairo_create(surf);
    gdk_cairo_set_source_window(cr, win, 0, 0);
    cairo_paint(cr);
    cairo_destroy(cr);

    const char *out = (const char *)path;
    cairo_surface_write_to_png(surf, out);
    cairo_surface_destroy(surf);

    // Сигнализируем патчеру через stdout
    printf("READY:%s\n", out);
    fflush(stdout);
}

gboolean do_shot(gpointer data) {
    // Повторный скриншот каждые 2 секунды
    // wv передаётся как data
    WebKitWebView *wv = (WebKitWebView *)data;

    GtkWidget *top = gtk_widget_get_toplevel(GTK_WIDGET(wv));
    GdkWindow *win = gtk_widget_get_window(top);
    if (!win) return G_SOURCE_CONTINUE;

    int w = gdk_window_get_width(win);
    int h = gdk_window_get_height(win);
    if (w <= 0 || h <= 0) return G_SOURCE_CONTINUE;

    // Получаем путь из user_data через глобал (упрощение)
    const char *out_path = (const char *)g_object_get_data(G_OBJECT(wv), "out_path");
    if (!out_path) return G_SOURCE_CONTINUE;

    cairo_surface_t *surf = cairo_image_surface_create(CAIRO_FORMAT_ARGB32, w, h);
    cairo_t *cr = cairo_create(surf);
    gdk_cairo_set_source_window(cr, win, 0, 0);
    cairo_paint(cr);
    cairo_destroy(cr);

    cairo_surface_write_to_png(surf, out_path);
    cairo_surface_destroy(surf);

    printf("READY:%s\n", out_path);
    fflush(stdout);

    return G_SOURCE_CONTINUE;
}
*/
import "C"
import (
	"fmt"
	"os"
	"unsafe"
)

func main() {
	if len(os.Args) < 4 {
		fmt.Fprintln(os.Stderr, "usage: webview_helper <url> <out.png> <width> <height>")
		os.Exit(1)
	}

	url := os.Args[1]
	outPath := os.Args[2]
	width := 400
	height := 300
	fmt.Sscanf(os.Args[3], "%d", &width)
	if len(os.Args) > 4 {
		fmt.Sscanf(os.Args[4], "%d", &height)
	}

	C.gtk_init(nil, nil)

	offscreen := C.gtk_offscreen_window_new()
	C.gtk_widget_set_size_request(offscreen, C.int(width), C.int(height))

	wv := C.webkit_web_view_new()
	C.gtk_widget_set_size_request(wv, C.int(width), C.int(height))
	C.gtk_container_add((*C.GtkContainer)(unsafe.Pointer(offscreen)), wv)

	cOut := C.CString(outPath)
	C.g_object_set_data((*C.GObject)(unsafe.Pointer(wv)),
		C.CString("out_path"), C.gpointer(unsafe.Pointer(cOut)))

	cURL := C.CString(url)
	C.webkit_web_view_load_uri((*C.WebKitWebView)(unsafe.Pointer(wv)), cURL)

	C.g_signal_connect_data(
		C.gpointer(unsafe.Pointer(wv)),
		C.CString("load-changed"),
		C.GCallback(C.cb_load),
		C.gpointer(unsafe.Pointer(cOut)),
		nil, 0,
	)

	C.g_timeout_add(2000, C.GSourceFunc(C.do_shot), C.gpointer(unsafe.Pointer(wv)))

	C.gtk_widget_show_all(offscreen)
	C.gtk_main()
}