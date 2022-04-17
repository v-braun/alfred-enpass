package imgcache

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path"
	"strings"

	aw "github.com/deanishe/awgo"
	"github.com/samber/lo"
)

type ImageCacheStatus string

const ImageCacheStatusSkip = "skip"
const ImageCacheStatusNone = "none"
const ImageCacheStatusExist = "exist"

type ImageCacheEntry struct {
	ItemId string
	Status ImageCacheStatus
	Fav    string
}

type ImageCacheRepo struct {
	Index        []*ImageCacheEntry
	wf           *aw.Workflow
	cacheRootDir string
}

func NewRepo(wf *aw.Workflow) *ImageCacheRepo {
	repo := &ImageCacheRepo{
		Index: []*ImageCacheEntry{},
		wf:    wf,
	}

	cacheRootDir := ensureImageCacheExist(wf)
	repo.cacheRootDir = cacheRootDir
	repo.Index = ensureImageIndexExists(wf)

	return repo
}

func (r *ImageCacheRepo) CacheImages() {
	if r.cacheRootDir == "" {
		fmt.Println("ERR", "no cache dir")
		return
	}

	items := lo.Filter(r.Index, func(v *ImageCacheEntry, i int) bool {
		if v.Fav == "" {
			return false
		}
		if v.Status != ImageCacheStatusNone {
			return false
		}

		return true
	})
	fmt.Println("INFO", "check items", len(items))

	for _, item := range items {
		data, err := downloadImgFile(item.Fav)
		if err != nil {
			item.Status = ImageCacheStatusSkip
			continue
		}

		_, err = createImgFile(r.wf, item.ItemId, data)
		if err != nil {
			item.Status = ImageCacheStatusSkip
			continue
		}

		item.Status = ImageCacheStatusExist

		r.StoreIndexFile()
	}

	fmt.Println("INFO", "store index file")
	r.StoreIndexFile()
}

func (r *ImageCacheRepo) StoreIndexFile() {
	fileContent, err := json.Marshal(r.Index)
	if err != nil {
		return
	}

	indexPath := path.Join(r.wf.Cache.Dir, "images", "index.json")
	os.WriteFile(indexPath, fileContent, 0644)
}

func (r *ImageCacheRepo) SetFavFor(id string, rawJson string) {
	if r.cacheRootDir == "" {
		return
	}
	if rawJson == "" {
		return
	}

	item, _ := lo.Find(r.Index, func(t *ImageCacheEntry) bool {
		return t.ItemId == id
	})

	if item == nil {
		item = &ImageCacheEntry{
			ItemId: id,
			Status: ImageCacheStatusNone,
			Fav:    "",
		}

		r.Index = append(r.Index, item)
	}

	if item.Fav != "" {
		return
	}

	imgKey := getImageKey(rawJson)
	if imgKey == "" {
		return
	}

	item.Fav = imgKey
}

func (r *ImageCacheRepo) GetImagePath(id string) string {
	result, _ := lo.Find(r.Index, func(t *ImageCacheEntry) bool {
		return t.ItemId == id && t.Status == ImageCacheStatusExist
	})

	if result == nil {
		return ""
	}

	filePath := path.Join(r.wf.Cache.Dir, "images", id+".png")
	_, err := os.Stat(filePath)
	if err == nil {
		return filePath
	}

	return ""
}

func createImgFile(wf *aw.Workflow, id string, data []byte) (string, error) {
	filePath := path.Join(wf.Cache.Dir, "images", id+".png")
	return filePath, ioutil.WriteFile(filePath, data, 0644)
}

func ensureImageCacheExist(wf *aw.Workflow) string {
	imgCacheDir := path.Join(wf.Cache.Dir, "images")
	_, err := os.Stat(imgCacheDir)
	if err == nil {
		return imgCacheDir
	}
	if errors.Is(err, os.ErrNotExist) {
		if os.MkdirAll(imgCacheDir, 0740) == nil {
			return imgCacheDir
		}

		return ""
	}

	return ""
}

func getImageKey(rawJson string) string {
	if rawJson == "" {
		return ""
	}

	var data interface{}
	err := json.Unmarshal([]byte(rawJson), &data)
	if err != nil {
		log.Println("could not unmarshal imgKey", err.Error())
		return ""
	}

	jsonPayload, ok := data.(map[string]interface{})
	if !ok {
		log.Println("could not cast imgKey", err.Error())
		return ""
	}

	val, ok := jsonPayload["fav"]
	if !ok || val == "" {
		return ""
	}

	fav, ok := val.(string)
	if !ok || fav == "" {
		return ""
	}

	return fav
}

func ensureImageIndexExists(wf *aw.Workflow) []*ImageCacheEntry {
	indexPath := path.Join(wf.Cache.Dir, "images", "index.json")
	_, err := os.Stat(indexPath)
	if errors.Is(err, os.ErrNotExist) {
		ioutil.WriteFile(indexPath, []byte("[]"), 0644)
	}

	indexData, err := ioutil.ReadFile(indexPath)
	if err != nil {
		log.Println("ERR", "Failed read index file", err)
		return []*ImageCacheEntry{}
	}

	result := make([]*ImageCacheEntry, 0)
	err = json.Unmarshal(indexData, &result)
	if err != nil {
		log.Println("ERR", "Failed parse json", err)
		return []*ImageCacheEntry{}
	}

	log.Println("INFO", "readed file", indexPath, "items", len(result))
	return result
}

func downloadImgFile(imgKey string) ([]byte, error) {
	parts := strings.Split(imgKey, ".")
	domains := make([]string, 0)
	for {
		fullUrl := strings.Join(parts, ".")
		domains = append(domains, fmt.Sprintf("https://favicon.enpass.io/websites/%s/120x120.png", fullUrl))
		domains = append(domains, fmt.Sprintf("https://favicon.enpass.io/websites/%s/120x120-1.png", fullUrl))
		domains = append(domains, fmt.Sprintf("https://favicon.enpass.io/websites/%s/120x120-2.png", fullUrl))
		domains = append(domains, fmt.Sprintf("https://favicon.enpass.io/websites/%s/120x120-3.png", fullUrl))

		parts = parts[1:]
		if len(parts) < 2 {
			break
		}
	}

	imgData, err := downloadImg(domains)

	return imgData, err
}

func downloadImg(domains []string) ([]byte, error) {
	var err error = nil
	for _, domain := range domains {
		reqUrl := domain
		res, err := http.Get(reqUrl)
		if err != nil {
			continue
		}
		if res.StatusCode != 200 {
			continue
		}

		imgData, err := ioutil.ReadAll(res.Body)
		if err != nil {
			continue
		}

		return imgData, nil
	}

	return nil, err
}

// "https://favicon.enpass.io/websites/%s/120x120-3.png"
