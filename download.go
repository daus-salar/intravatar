package main

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
)

type Request struct {
	hash string
	size int
	dflt string
}

const (
	d_404 = "404"
)

func readFromFile(filename string, size int) *Avatar {
	file, err := os.Open(filename)
	if err != nil {
		log.Printf("Error reading file: %v", err)
		return nil
	}
	defer file.Close()
	avatar := readImage(file)
	err = scale(avatar, size)
	if err != nil {
		log.Printf("Could not scale image: %v", err)
		return nil // don't return the image, if we can't scale it it is probably corrupt
	}

	avatar.cacheControl = "max-age=300"
	if info, e := os.Stat(filename); e == nil {
		timestamp := info.ModTime().UTC()
		avatar.lastModified = timestamp.Format(http.TimeFormat)
	} else {
		avatar.lastModified = "Sat, 1 Jan 2000 12:00:00 GMT"
	}
	return avatar
}

func retrieveFromLocal(request Request) *Avatar {
	return readFromFile(createAvatarPath(request.hash), request.size)
}

// Retrieves the avatar from the remote service, returning nil if there is no avatar or it could not be retrieved
// dflt is used instead of request.dflt
func retrieveFromRemoteUrl(remoteUrl string, request Request, dflt string) *Avatar {
	options := fmt.Sprintf("s=%d", request.size)
//	dflt := remoteDefault
//	if request.dflt != "" {
//		dflt = request.dflt
//	}
//	if dflt != "" {
//		options += "&d=" + dflt
//	}
	remote := remoteUrl + "/" + request.hash + "?" + options
	log.Printf("Retrieving from: %s", remote)
	resp, err2 := http.Get(remote)
	if err2 != nil {
		log.Printf("Remote lookup of %s failed with error: %s", remote, err2)
		return nil
	}
	if resp.StatusCode == 404 {
		log.Printf("Avatar not found on remote")
		return nil
	}
	avatar := readImage(resp.Body)
	avatar.size = request.size // assume image is scaled by remote service
	avatar.cacheControl = resp.Header.Get("Cache-Control")
	avatar.lastModified = resp.Header.Get("Last-Modified")
	return avatar
}

// Retrieves the avatar from the remote services, returning nil if there is no avatar or it could not be retrieved
func retrieveFromRemote(request Request) *Avatar {
	l := len(remoteUrls)
	if l == 0 {
		return nil
	}
	for _, remoteUrl := range remoteUrls[:l-1] {
		if avatar := retrieveFromRemoteUrl(remoteUrl, request, d_404); avatar != nil {
			return avatar
		}
	}
	return retrieveFromRemoteUrl(remoteUrls[l-1], request, request.dflt)
}

func writeAvatarResult(w http.ResponseWriter, avatar *Avatar) {
	setHeaderField(w, "Last-Modified", avatar.lastModified)
	setHeaderField(w, "Cache-Control", avatar.cacheControl)
	b := bytes.NewBuffer(avatar.data)
	_, err := io.Copy(w, b)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func retrieveImage(request Request, w http.ResponseWriter, r *http.Request) *Avatar {
	avatar := retrieveFromLocal(request)
	if avatar == nil {
		avatar = retrieveFromRemote(request)
	}
	if avatar == nil && request.dflt != d_404 {
		avatar = readFromFile(defaultImage, request.size)
	}
	if avatar == nil && request.dflt != d_404 {
		avatar = readFromFile("resources/mm", request.size)
	}
	return avatar
}

func loadImage(request Request, w http.ResponseWriter, r *http.Request) {
	log.Printf("Loading image: %v", request)
	avatar := retrieveImage(request, w, r)
	if avatar == nil {
		http.NotFound(w, r)
	} else {
		writeAvatarResult(w, avatar)
	}
}

// checks if dflt is a valid default image and only then returns it
// otherwise an empty string is returned
func validDefault(dflt string) string {
	if dflt == d_404 {
		return dflt
	}
	return ""
}

func avatarHandler(w http.ResponseWriter, r *http.Request, hash string) {
	r.ParseForm()
	sizeParam := r.FormValue("s")
	size := 80
	if sizeParam != "" {
		if s, err := strconv.Atoi(sizeParam); err == nil {
			size = max(min(s, maxSize), minSize) 
		}
	}
	dflt := validDefault(r.FormValue("d"))
	
	loadImage(Request{hash: hash, size: size, dflt: dflt}, w, r)
}
