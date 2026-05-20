//go:build linux

package main

/*
#cgo pkg-config: gtk+-3.0 webkit2gtk-4.1 glib-2.0
#include <gtk/gtk.h>
#include <webkit2/webkit2.h>
#include <cairo.h>
#include <stdlib.h>
#include <string.h>

typedef struct {
    char *url;
    int   width;
    int   height;
    char *out_path;
    char *module_id;
} ShotArgs;

typedef struct {
    GtkWidget *offscreen;
    GtkWidget *wv;
    ShotArgs  *args;
    guint      timer_id;
} ShotState;

// Объявляем Go функцию — она будет доступна из C
extern void goScreenshotReady(char *path, char *id);

static void save_and_notify(ShotState *st) {
    GdkWindow *gdk_win = gtk_widget_get_window(st->offscreen);
    if (!gdk_win) return;

    int w = gtk_widget_get_allocated_width(st->offscreen);
    int h = gtk_widget_get_allocated_height(st->offscreen);
    if (w <= 0 || h <= 0) return;

    cairo_surface_t *surf = cairo_image_surface_create(CAIRO_FORMAT_ARGB32, w, h);
    cairo_t *cr = cairo_create(surf);
    gdk_cairo_set_source_window(cr, gdk_win, 0, 0);
    cairo_paint(cr);
    cairo_destroy(cr);

    cairo_surface_write_to_png(surf, st->args->out_path);
    cairo_surface_destroy(surf);

    // Вызываем Go колбек
    goScreenshotReady(st->args->out_path, st->args->module_id);
}

static gboolean on_timer(gpointer data) {
    save_and_notify((ShotState *)data);
    return G_SOURCE_CONTINUE;
}

static void on_load_changed(WebKitWebView *wv,
                             WebKitLoadEvent ev,
                             gpointer data) {
    if (ev != WEBKIT_LOAD_FINISHED) return;
    ShotState *st = (ShotState *)data;
    save_and_notify(st);
    if (st->timer_id == 0) {
        st->timer_id = g_timeout_add(2000, on_timer, st);
    }
}

static gboolean do_headless(gpointer data) {
    ShotArgs *a = (ShotArgs *)data;

    GtkWidget *offscreen = gtk_offscreen_window_new();
    gtk_widget_set_size_request(offscreen, a->width, a->height);

    GtkWidget *wv = webkit_web_view_new();
    gtk_widget_set_size_request(wv, a->width, a->height);
    gtk_container_add(GTK_CONTAINER(offscreen), wv);

    ShotState *st = g_new0(ShotState, 1);
    st->offscreen = offscreen;
    st->wv        = wv;
    st->args      = a;
    st->timer_id  = 0;

    webkit_web_view_load_uri(WEBKIT_WEB_VIEW(wv), a->url);
    g_signal_connect(wv, "load-changed", G_CALLBACK(on_load_changed), st);

    gtk_widget_show_all(offscreen);
    return G_SOURCE_REMOVE;
}

void start_headless(const char *url, int w, int h,
                    const char *out_path, const char *module_id) {
    ShotArgs *a   = g_new0(ShotArgs, 1);
    a->url        = strdup(url);
    a->width      = w > 0 ? w : 400;
    a->height     = h > 0 ? h : 300;
    a->out_path   = strdup(out_path);
    a->module_id  = strdup(module_id);
    g_idle_add(do_headless, a);
}

// ── Браузер в отдельном окне ─────────────────────────────────

static void cb_load_uri(WebKitWebView *wv, WebKitLoadEvent ev, gpointer entry) {
    const char *uri = webkit_web_view_get_uri(wv);
    if (uri) gtk_entry_set_text(GTK_ENTRY(entry), uri);
}
static void cb_url_activate(GtkEntry *e, gpointer wv) {
    webkit_web_view_load_uri(WEBKIT_WEB_VIEW(wv), gtk_entry_get_text(e));
}
static void cb_back(GtkButton *b, gpointer wv)    { webkit_web_view_go_back(WEBKIT_WEB_VIEW(wv)); }
static void cb_forward(GtkButton *b, gpointer wv) { webkit_web_view_go_forward(WEBKIT_WEB_VIEW(wv)); }
static void cb_reload(GtkButton *b, gpointer wv)  { webkit_web_view_reload(WEBKIT_WEB_VIEW(wv)); }

typedef struct { char *url; int width; int height; } OpenArgs;

static gboolean do_open_browser(gpointer data) {
    OpenArgs *a = (OpenArgs *)data;

    GtkWidget *win = gtk_window_new(GTK_WINDOW_TOPLEVEL);
    gtk_window_set_title(GTK_WINDOW(win), a->url);
    gtk_window_set_default_size(GTK_WINDOW(win), a->width, a->height);
    g_signal_connect(win, "destroy", G_CALLBACK(gtk_widget_destroy), NULL);

    GtkWidget *vbox   = gtk_box_new(GTK_ORIENTATION_VERTICAL, 0);
    GtkWidget *navbar = gtk_box_new(GTK_ORIENTATION_HORIZONTAL, 4);
    GtkWidget *btn_b  = gtk_button_new_with_label("←");
    GtkWidget *btn_f  = gtk_button_new_with_label("→");
    GtkWidget *btn_r  = gtk_button_new_with_label("↺");
    GtkWidget *urlbar = gtk_entry_new();
    gtk_entry_set_text(GTK_ENTRY(urlbar), a->url);

    gtk_box_pack_start(GTK_BOX(navbar), btn_b,  FALSE, FALSE, 2);
    gtk_box_pack_start(GTK_BOX(navbar), btn_f,  FALSE, FALSE, 2);
    gtk_box_pack_start(GTK_BOX(navbar), btn_r,  FALSE, FALSE, 2);
    gtk_box_pack_start(GTK_BOX(navbar), urlbar, TRUE,  TRUE,  2);
    gtk_box_pack_start(GTK_BOX(vbox), navbar,   FALSE, FALSE, 4);

    GtkWidget *wv = webkit_web_view_new();
    gtk_box_pack_start(GTK_BOX(vbox), wv, TRUE, TRUE, 0);
    gtk_container_add(GTK_CONTAINER(win), vbox);
    webkit_web_view_load_uri(WEBKIT_WEB_VIEW(wv), a->url);

    g_signal_connect(wv,     "load-changed", G_CALLBACK(cb_load_uri),    urlbar);
    g_signal_connect(urlbar, "activate",     G_CALLBACK(cb_url_activate), wv);
    g_signal_connect(btn_b,  "clicked",      G_CALLBACK(cb_back),         wv);
    g_signal_connect(btn_f,  "clicked",      G_CALLBACK(cb_forward),      wv);
    g_signal_connect(btn_r,  "clicked",      G_CALLBACK(cb_reload),       wv);

    gtk_widget_show_all(win);
    free(a->url);
    g_free(a);
    return G_SOURCE_REMOVE;
}

void open_browser(const char *url, int w, int h) {
    OpenArgs *a = g_new0(OpenArgs, 1);
    a->url    = strdup(url);
    a->width  = w > 0 ? w : 1000;
    a->height = h > 0 ? h : 700;
    g_idle_add(do_open_browser, a);
}
*/
import "C"

import (
	"fmt"
	"os"
	"sync"
	"unsafe"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
)

var (
	moduleImageCache = map[string]*canvas.Image{}
	moduleImageMu    sync.Mutex
)

// goScreenshotReady — экспортируется в C через //export
//
//export goScreenshotReady
func goScreenshotReady(cPath *C.char, cID *C.char) {
	path := C.GoString(cPath)
	id := C.GoString(cID)

	moduleImageMu.Lock()
	img, ok := moduleImageCache[id]
	moduleImageMu.Unlock()

	if !ok || img == nil {
		return
	}

	img.File = path
	canvas.Refresh(img)
}

// StartModuleWebView запускает headless WebKit, возвращает canvas.Image
// который обновляется скриншотами страницы каждые 2 секунды
func StartModuleWebView(id, rawURL string, width, height int) *canvas.Image {
	if width <= 0 {
		width = 300
	}
	if height <= 0 {
		height = 200
	}

	outPath := fmt.Sprintf("%s/patcher_module_%s.png", os.TempDir(), id)

	img := canvas.NewImageFromFile(outPath)
	img.FillMode = canvas.ImageFillStretch
	img.Resize(fyne.NewSize(float32(width), float32(height)))

	moduleImageMu.Lock()
	moduleImageCache[id] = img
	moduleImageMu.Unlock()

	cURL := C.CString(rawURL)
	cOut := C.CString(outPath)
	cID := C.CString(id)
	defer C.free(unsafe.Pointer(cURL))
	defer C.free(unsafe.Pointer(cOut))
	defer C.free(unsafe.Pointer(cID))

	C.start_headless(cURL, C.int(width), C.int(height), cOut, cID)

	return img
}

// OpenEmbeddedWebView открывает браузер в отдельном GTK окне
func OpenEmbeddedWebView(rawURL string, width, height int) {
	cu := C.CString(rawURL)
	defer C.free(unsafe.Pointer(cu))
	C.open_browser(cu, C.int(width), C.int(height))
}