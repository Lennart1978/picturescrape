package main

import (
	"fmt"
	"image/gif"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/gocolly/colly"
)

// Set maximum of images to load
const maxImages = 500

// Icon and Logo
var imgIcon = canvas.NewImageFromResource(resourceIconPng)
var imgLogo = canvas.NewImageFromResource(resourceLogoPng)

// Global picture cache
var imageCache = make(map[string]*canvas.Image)

// Load the image in a Goroutine and cache it
func getCachedImage(url string) *canvas.Image {
	if img, found := imageCache[url]; found {
		return img
	}

	imageChan := make(chan *canvas.Image)

	go func(url string) {
		res, err := fyne.LoadResourceFromURLString(url)
		if err != nil {
			log.Printf("Failed to load resource from URL: %v", err)
			imageChan <- nil
			return
		}

		img := canvas.NewImageFromResource(res)
		img.FillMode = canvas.ImageFillContain
		imageCache[url] = img
		imageChan <- img
	}(url)

	// Wait for Goroutine...
	img := <-imageChan
	if img == nil {
		// If an error occurred, return a placeholder image
		placeholder := canvas.NewImageFromResource(resourceIconPng)
		placeholder.FillMode = canvas.ImageFillContain
		return placeholder
	}

	return img
}

func main() {
	// Icon and Logo size and fill mode
	imgIcon.FillMode = canvas.ImageFillContain
	imgIcon.SetMinSize(fyne.NewSize(64, 64))
	imgLogo.FillMode = canvas.ImageFillContain
	imgLogo.SetMinSize(fyne.NewSize(223, 100))

	var (
		urlAll               string
		pictures             []string          //Holds all scraped pictures
		imageMinX, imageMinY float32  = 16, 16 //Minimum size of images
	)

	a := app.NewWithID("com.lennart.picturescrape")
	w := a.NewWindow("Picturescrape")
	w.Resize(fyne.NewSize(300, 600))
	w.CenterOnScreen()
	w.SetFixedSize(true)
	a.SetIcon(resourceIconPng)

	// Label
	labelUrl := widget.NewLabel("⭐ URL:")
	labelUrl.Alignment = fyne.TextAlignCenter
	labelCnt := widget.NewLabel("")
	labelCnt.Alignment = fyne.TextAlignCenter
	labelCnt.TextStyle.Bold = true
	labelCnt.TextStyle.Italic = true

	// About Labels
	labelAbout1 := widget.NewLabel("(c)2023 by Lennart Martens")
	labelAbout1.Alignment = fyne.TextAlignCenter
	labelAbout1.TextStyle.Italic = true
	labelAbout2 := widget.NewLabel("Version 1.0")
	labelAbout2.TextStyle.Bold = true
	labelAbout2.Alignment = fyne.TextAlignCenter
	githubURL, _ := url.Parse("https://github.com/lennart1978/picturescrape")
	linkAbout := widget.NewHyperlink("www.github.com/lennart1978/picturescrape", githubURL)
	linkAbout.Alignment = fyne.TextAlignCenter
	labelAbout4 := widget.NewLabel("License: MIT")
	labelAbout4.Alignment = fyne.TextAlignCenter
	labelAbout5 := widget.NewLabel("Powered by Colly, Golang and Fyne (www.fyne.io)")
	labelAbout5.Alignment = fyne.TextAlignCenter
	labelAbout5.TextStyle.Italic = true

	// URL Entry
	entryUrl := widget.NewEntry()
	entryUrl.SetPlaceHolder("https://example.com")

	// Picture list
	list := widget.NewList(nil, nil, nil)
	list.Length = func() int {
		return len(pictures)
	}
	list.CreateItem = func() fyne.CanvasObject {
		img := canvas.NewImageFromResource(nil)
		img.SetMinSize(fyne.NewSize(imageMinX, imageMinY))
		cont := container.NewStack(img)
		return cont
	}
	list.UpdateItem = func(index widget.ListItemID, item fyne.CanvasObject) {
		imgContainer, ok := item.(*fyne.Container)
		if !ok {
			log.Fatal("item is not a *fyne.Container")
			return
		}

		urlL := pictures[index]
		if strings.HasSuffix(urlL, ".gif") {
			// if image is .gif use loadGifImage() function
			gifImage, err := loadGifImage(urlL)
			if err != nil {
				log.Fatal("Failed to load GIF image:", err)
			}
			gifImage.FillMode = canvas.ImageFillContain
			imgContainer.Objects = []fyne.CanvasObject{gifImage}
		} else {
			// For all other images use getCachedImage() function
			img := getCachedImage(urlL)

			img.FillMode = canvas.ImageFillContain
			imgContainer.Objects = []fyne.CanvasObject{img}
		}

		imgContainer.Refresh()
	}
	// Download selected image
	list.OnSelected = func(id widget.ListItemID) {
		downloadPic(pictures[id], &w)
	}

	// Slider
	slider := widget.NewSlider(16, 128)
	slider.Step = 8
	slider.OnChanged = func(v float64) {
		imageMinX = float32(v)
		imageMinY = float32(v)
		list.Refresh()
	}

	// Toolbar
	toolbar := widget.NewToolbar(
		widget.NewToolbarAction(theme.SearchIcon(), func() {
			pictures = nil
			list.Refresh()
			urlAll = entryUrl.Text
			urlAll = ensureProtocol(urlAll)
			dmn := getDomain(urlAll)
			var err error
			pictures, err = scrapePictures(dmn, urlAll)
			if err != nil {
				fmt.Println("Fehler bei Scrapen! :", err)
			}
			list.Refresh()
			labelCnt.SetText(fmt.Sprintf("Found %d pictures", len(pictures)))
		}),
		widget.NewToolbarAction(theme.DocumentSaveIcon(), func() {
			if len(pictures) > 0 {
				go func() {
					downloadAll(pictures, &w)
				}()
			} else {
				dialog.ShowInformation("Information", "No pictures found", w)
			}

		}),
		widget.NewToolbarAction(theme.DeleteIcon(), func() {
			pictures = nil
			list.Refresh()
			labelCnt.SetText("")
			entryUrl.SetText("")
		}),
		widget.NewToolbarAction(theme.SettingsIcon(), func() {
			dlg := dialog.NewCustom("⏳ Image size < - >", "set!", slider, w)
			dlg.Show()
		}),
		widget.NewToolbarSpacer(),
		widget.NewToolbarAction(theme.HelpIcon(), func() {
			contAbout := container.NewVBox(
				imgLogo,
				labelAbout1,
				labelAbout2,
				linkAbout,
				labelAbout5,
				labelAbout4,
			)
			dlg := dialog.NewCustom("About", "cool!", contAbout, w)
			dlg.Show()
		}),
	)

	// Buttons
	buttonScrape := widget.NewButton("🌀 Scrape", func() {
		pictures = nil
		list.Refresh()
		urlAll = entryUrl.Text
		urlAll = ensureProtocol(urlAll) // Make sure the URL contains a protocol
		dmn := getDomain(urlAll)
		var err error
		pictures, err = scrapePictures(dmn, urlAll)
		if err != nil {
			fmt.Println("Error while scraping!:", err)
		}
		list.Refresh()
		labelCnt.SetText(fmt.Sprintf("Found %d pictures", len(pictures)))
	})
	buttonClear := widget.NewButton("❌ Clear", func() {
		pictures = nil
		list.Refresh()
		labelCnt.SetText("")
		entryUrl.SetText("")
	})
	buttonDwnld := widget.NewButton("✅ Download all", func() {
		if len(pictures) > 0 {
			go func() {
				downloadAll(pictures, &w)
			}()
		} else {
			dialog.ShowInformation("Information", "⁉️ No pictures found", w)
		}
	})
	buttonExit := widget.NewButton("⛔ Exit", func() {
		dlg := dialog.NewConfirm("⛔ Exit", "Do you really want to leave ?", func(b bool) {
			if b {
				a.Quit()
				os.Exit(0)
			}
		}, w)
		dlg.Show()
	})

	// Containers
	contUp := container.NewVBox(toolbar, labelUrl, entryUrl, labelCnt)
	contBttm := container.NewHBox(buttonScrape, buttonClear, buttonDwnld, buttonExit)
	cont := container.NewBorder(contUp, contBttm, nil, nil, list)

	w.SetContent(cont)

	w.ShowAndRun()

}

// Download all pictures
func downloadAll(pictures []string, w *fyne.Window) {

	// File dialog
	dirDialog := dialog.NewFolderOpen(func(dir fyne.ListableURI, err error) {
		if err != nil || dir == nil {
			return
		}

		for _, picURL := range pictures {
			// Extract filename from URL
			u, err := url.Parse(picURL)
			if err != nil {
				fmt.Println("Failed to parse URL:", err)
				continue // jump to next picture
			}
			fileName := filepath.Base(u.Path)
			if fileName == "." || fileName == "/" {
				fileName = "unknown.jpg" // Standard name if can't get filename
			}

			// Full path to file
			filePath := filepath.Join(dir.Path(), fileName)

			// Send HTTP query to download the picture
			resp, err := http.Get(picURL)
			if err != nil {
				fmt.Println("Failed to download image:", err)
				continue
			}

			// Datei zum Schreiben öffnen
			file, err := os.Create(filePath)
			if err != nil {
				fmt.Println("Failed to create file:", err)
				resp.Body.Close()
				continue
			}

			// Save downloaded picture to file
			_, err = io.Copy(file, resp.Body)
			if err != nil {
				fmt.Println("Failed to save image:", err)
				file.Close()
				resp.Body.Close()
				continue
			}

			// Release resources
			file.Close()
			resp.Body.Close()

			// Change file permissions on Linux
			if runtime.GOOS == "linux" {
				err = os.Chmod(filePath, 0665)
				if err != nil {
					fmt.Println("Failed to set file permissions:", err)
					continue
				}
			}
		}
	}, *w)
	dirDialog.Show()
}

func downloadPic(URL string, w *fyne.Window) {
	// Filedialog
	saveDialog := dialog.NewFileSave(func(writer fyne.URIWriteCloser, err error) {
		if writer == nil {
			return
		}

		if err != nil {
			dialog.ShowError(err, *w)
			return
		}
		defer writer.Close()

		// Send HTTP request to download the image file
		resp, err := http.Get(URL)
		if err != nil {
			dialog.ShowError(err, *w)
			return
		}
		defer resp.Body.Close()

		// Check whether the HTTP request was successful
		if resp.StatusCode != http.StatusOK {
			err = fmt.Errorf("failed to download image: %v", resp.Status)
			dialog.ShowError(err, *w)
			return
		}

		// Write the downloaded image file to the selected file
		_, err = io.Copy(writer, resp.Body)
		if err != nil {
			dialog.ShowError(err, *w)
			return
		}

		// Change file permissions on Linux
		if runtime.GOOS == "linux" {
			filePath := writer.URI().Path()
			err = os.Chmod(filePath, 0665)
			if err != nil {
				dialog.ShowError(err, *w)
				return
			}
		}

	}, *w)

	// Set suggested file name. Extract the file name from the URL
	u, err := url.Parse(URL)
	if err != nil {
		fmt.Println("Failed to parse URL:", err)
	}

	fileName := filepath.Base(u.Path)
	if fileName == "." || fileName == "/" {
		fileName = "unknown.jpg" // Default name if the file name cannot be determined
	}

	saveDialog.SetFileName(fileName)

	// who would have thought: Show the dialog !!! :-)
	saveDialog.Show()
}

func ensureProtocol(urlStr string) string {
	if strings.HasPrefix(urlStr, "http://") || strings.HasPrefix(urlStr, "https://") {
		return urlStr
	}

	httpsURL := "https://" + urlStr
	httpURL := "http://" + urlStr

	// Tiemout for HTTP request
	client := http.Client{
		Timeout: 2 * time.Second,
	}

	// First try to reach the HTTPS version of the URL
	resp, err := client.Get(httpsURL)
	if err == nil && resp.StatusCode == http.StatusOK {
		return httpsURL
	}

	// If HTTPS fails, try HTTP
	resp, err = client.Get(httpURL)
	if err == nil && resp.StatusCode == http.StatusOK {
		return httpURL
	}

	// If both fail, return the original URL
	return urlStr
}

// Extracts the domain from URL
func getDomain(urlStr string) string {
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		fmt.Println("Error parsing URL:", err)
		return ""
	}

	domain := parsedURL.Hostname()
	return domain
}

// Load a gif image and return *canvas.image
func loadGifImage(url string) (*canvas.Image, error) {
	// Send HTTP request to download the image file
	client := &http.Client{
		Timeout: time.Second * 10, // 10 Seconds timeout
	}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP request failed with status code %d", resp.StatusCode)
	}

	if resp.Header.Get("Content-Type") != "image/gif" {
		return nil, fmt.Errorf("expected image/gif content type, got %s", resp.Header.Get("Content-Type"))
	}

	// Decode GIF image
	gifImage, err := gif.Decode(resp.Body)
	if err != nil {
		return nil, err
	}

	// Create a Fyne image from the decoded image
	fyneImage := canvas.NewImageFromImage(gifImage)
	return fyneImage, nil
}

func scrapePictures(domain, page string) ([]string, error) {
	var pictureSrc []string

	// Extract the protocol from URL
	protocol := "http"
	if strings.HasPrefix(page, "https") {
		protocol = "https"
	}

	collector := colly.NewCollector(
		colly.AllowedDomains(domain),
	)

	collector.UserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/64.0.3282.140 Safari/537.36 Edge/17.17134"

	// Handler for td-Tags with background images
	collector.OnHTML("td[style]", func(element *colly.HTMLElement) {
		style := element.Attr("style")
		re := regexp.MustCompile(`url\((.*?)\)`)
		matches := re.FindStringSubmatch(style)
		if len(matches) > 1 {
			src := fmt.Sprintf("%s://%s/%s", protocol, domain, matches[1])
			pictureSrc = append(pictureSrc, src)
		}
	})

	// Handler for img-Tags
	collector.OnHTML("img", func(element *colly.HTMLElement) {
		src := element.Attr("src")
		if strings.HasPrefix(src, "//") {
			// Protokoll-relativer Link
			src = fmt.Sprintf("%s:%s", protocol, src)
		} else if !strings.HasPrefix(src, "http") {
			if strings.Contains(src, domain) {
				// Relativer Link mit Domain
				src = fmt.Sprintf("%s://%s", protocol, src)
			} else {
				// Absoluter oder relativer Pfad ohne Domain
				src = fmt.Sprintf("%s://%s/%s", protocol, domain, src)
			}
		}
		// Verify that the URL is a valid image URL before adding it to the slice
		if isValidImageURL(src) {
			pictureSrc = append(pictureSrc, src)
		}
	})

	collector.OnRequest(func(request *colly.Request) {
		fmt.Println("Visiting", request.URL.String())
	})

	if len(pictureSrc) >= maxImages {
		return pictureSrc, nil
	}

	err := collector.Visit(page)
	if err != nil {
		return pictureSrc, err
	}

	// find, delete duplicates
	pictureSrc, err = removeDuplicates(pictureSrc)
	if err != nil {
		return pictureSrc, err
	}
	return pictureSrc, nil
}

// Function to remove duplicates from a slice of strings
func removeDuplicates(pictureSrc []string) ([]string, error) {
	seen := make(map[string]bool)
	result := []string{}
	for _, src := range pictureSrc {
		if !seen[src] {
			seen[src] = true
			result = append(result, src)
		}
	}
	return result, nil
}

// Function to check if the URL points to a valid image file
func isValidImageURL(url string) bool {
	validExtensions := []string{".png", ".jpg", ".jpeg", ".gif", ".bmp", ".svg"}
	// Check whether the URL contains one of the valid extensions
	for _, ext := range validExtensions {
		if strings.HasSuffix(strings.ToLower(url), ext) {
			return true
		}
	}
	return false
}
