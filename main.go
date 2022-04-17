package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path"
	"sort"
	"strings"
	"time"

	aw "github.com/deanishe/awgo"
	"github.com/pquerna/otp/totp"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
	"github.com/v-braun/alfred-enpass/imgcache"
	"github.com/v-braun/enpass-cli/pkg/enpass"

	"github.com/zalando/go-keyring"
)

type SetupMode = string

const SetupModeDbPath SetupMode = "dbpath"
const SetupModeDbPassword SetupMode = "password"
const SetupModeCommit SetupMode = "commit"

// const SetupModeCommitDbPath SetupMode = "commitdbpath"
// const SetupModeCommitDbPassword SetupMode = "commitpassword"

type WorkflowExecCtx struct {
	SetupMode     SetupMode `env:"setupMode"`
	EnpassFile    string    `env:"enpassFile"`
	TmpEnpassPass string    `env:"enpassPass"`

	PickedRootItem string `env:"pickedRootItem"`

	imgCacheRepo *imgcache.ImageCacheRepo
}

type EnPassEntry struct {
	id    string
	cards []enpass.Card
	title string
	ico   string
}

var (
	wf             *aw.Workflow
	setKey, getKey string
)

func init() {
	wf = aw.New()
	// flag.StringVar(&setKey, "set", "", "save a value")
	// flag.StringVar(&getKey, "get", "", "enter a new value")
}

func appendFile(filename, text string) {
	f, err := os.OpenFile(filename, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		panic(err)
	}

	defer f.Close()

	if _, err = f.WriteString(text); err != nil {
		panic(err)
	}
}

func run() {
	wf.Args()

	flag.Parse()

	log.Println("start app")
	// appendFile("/Users/vbr/tmp/alfred-test/tmp.log", fmt.Sprintf("start pid: %d", os.Getegid()))

	// time.Sleep(time.Second * 60)

	// Default configuration
	ctx := &WorkflowExecCtx{
		SetupMode:      "",
		EnpassFile:     "",
		TmpEnpassPass:  "",
		PickedRootItem: "",
		imgCacheRepo:   imgcache.NewRepo(wf),
	}

	// Update config from environment variables
	if err := wf.Config.To(ctx); err != nil {
		panic(err)
	}

	// ----------------------------------------------------------------
	// Parse command-line flags and decide what to do

	if handleUpdateCache(ctx) {
		return
	}

	defer func() {
		ctx.imgCacheRepo.StoreIndexFile()
	}()

	if handleSetupDbPath(ctx) {
		return
	}
	if handleSetupDbPass(ctx) {
		return
	}
	if handleSetupComplete(ctx) {
		return
	}
	if handleNeedSetup(ctx) {
		return
	}

	if handleSearchEntries(ctx) {
		defer func() {
			startBgCmd(wf)
		}()
		return
	}
	if handleSearchWithinEntry(ctx) {
		return
	}

	wf.WarnEmpty("No Matching Items", "Try a different query?")
	wf.SendFeedback()
}

func main() {
	wf.Run(run)
}

func handleNeedSetup(ctx *WorkflowExecCtx) bool {
	setupAndSendItem := func() bool {
		wf.NewItem("Setup Workflow").
			Subtitle("↩ to start setup").
			Valid(true).
			Var("setupMode", SetupModeDbPath)

		return sendFeedbackAndEnd()
	}

	pass, err := keyring.Get("alfred-enpass", "enpass")
	if err != nil {
		log.Println("ERR", "get keyring pass", err.Error())
		return setupAndSendItem()
	}

	if pass == "" {
		log.Println("WARN", "pass not found in keyring")
		return setupAndSendItem()
	}

	if ctx.EnpassFile == "" {
		log.Println("WARN", "enpass file not known")
		return setupAndSendItem()
	}

	vault := openAndAuthVault(ctx)
	if vault == nil {
		return setupAndSendItem()
	} else {
		vault.Close()
	}

	return false
}

func openAndAuthVault(ctx *WorkflowExecCtx) *enpass.Vault {
	vault, err := enpass.NewVault(ctx.EnpassFile, logrus.DebugLevel)
	if err != nil {
		log.Println("ERR", "could not open enpass vault", err.Error())
		return nil
	}

	pass, err := keyring.Get("alfred-enpass", "enpass")
	if err != nil {
		log.Println("ERR", "get keyring pass", err.Error())
		return nil
	}

	err = vault.Open(&enpass.VaultCredentials{Password: pass})
	if err != nil {
		log.Println("ERR", "could not decrypt enpass vault", err.Error())
		return nil
	}

	return vault
}

func sendFeedbackAndEnd() bool {
	wf.SendFeedback()
	return true
}

func handleSearchWithinEntry(ctx *WorkflowExecCtx) bool {
	if ctx.PickedRootItem == "" {
		return false
	}

	entries, err := getEntries(ctx)
	if err != nil {
		wf.NewItem(err.Error()).
			Subtitle("please check logs in alfred").
			Valid(false)

		return sendFeedbackAndEnd()
	}

	entry, _ := lo.Find(entries, func(t *EnPassEntry) bool {
		return t.id == ctx.PickedRootItem
	})

	if entry == nil {
		return true
	}

	for _, row := range entry.cards {
		if row.Category == "section" {
			continue
		}
		if row.RawValue == "" {
			continue
		}

		beautyType := beautyfiyType(row.Type)
		typeIco := typeIco(row.Type)
		title := strings.Trim(row.Label, " ")
		if title == "" {
			title = beautyType
		}

		rowValue := row.RawValue
		subTitle := row.Type
		if !row.Sensitive {
			subTitle = fmt.Sprintf("%s", row.RawValue)
		} else {
			subTitle = fmt.Sprintf("%s", "***********")
			rowValue, err = row.Decrypt()
			if err != nil {
				log.Println("ERR fail decrypt sensetive data", err.Error())
				rowValue = row.RawValue
			}
		}
		if row.Type == "totp" {
			totpInput := strings.ReplaceAll(row.RawValue, " ", "")
			code, _ := totp.GenerateCode(totpInput, time.Now())
			rowValue = code
			if code != "" && len(code) >= 6 {
				subTitle = fmt.Sprintf("%s %s", code[0:3], code[3:])
			}
		}

		wf.NewItem(fmt.Sprintf("%s", title)).
			Subtitle(fmt.Sprintf("%s", subTitle)).
			Icon(&aw.Icon{
				Value: typeIco,
				Type:  aw.IconTypeImage,
			}).
			Var("clipVal", rowValue).
			Var("pickedRootItem", "").
			Valid(true)
	}

	query := flag.Arg(0)
	if query != "" {
		wf.Filter(query)
	}
	wf.SendFeedback()

	return sendFeedbackAndEnd()
}

func beautyfiyType(t string) string {
	if t == "username" {
		return "Username"
	}
	if t == "email" {
		return "E-Mail"
	}
	if t == "totp" {
		return "One-time code"
	}
	if t == "url" {
		return "Website"
	}
	if t == "password" {
		return "Password"
	}
	if t == "text" {
		return "Text"
	}

	return t
}
func typeIco(t string) string {
	if t == "username" {
		return path.Join(wf.Data.Dir, "user.png")
	}
	if t == "email" {
		return path.Join(wf.Data.Dir, "mail.png")
	}
	if t == "totp" {
		return path.Join(wf.Data.Dir, "totp.png")
	}
	if t == "url" {
		return path.Join(wf.Data.Dir, "url.png")
	}
	if t == "password" {
		return path.Join(wf.Data.Dir, "pass.png")
	}
	if t == "text" {
		return path.Join(wf.Data.Dir, "unknown.png")
	}

	return path.Join(wf.Data.Dir, "unknown.png")
}

func handleSearchEntries(ctx *WorkflowExecCtx) bool {
	if ctx.PickedRootItem != "" {
		return false
	}

	entries, err := getEntries(ctx)
	if err != nil {
		wf.NewItem(err.Error()).
			Subtitle("please check logs in alfred").
			Valid(false)

		return sendFeedbackAndEnd()
	}

	for _, entry := range entries {
		matches := []string{strings.Trim(entry.title, " ")}
		for _, card := range entry.cards {
			matches = append(matches, strings.Trim(card.Label, " "))
		}

		ctx.imgCacheRepo.SetFavFor(entry.id, entry.ico)
		icoFilePath := ctx.imgCacheRepo.GetImagePath(entry.id)
		if icoFilePath == "" {
			icoFilePath = typeIco("")
		}

		wf.NewItem(fmt.Sprintf("%s", entry.title)).
			Subtitle(fmt.Sprintf("%d items", len(entry.cards))).
			// Match(strings.Join(matches, " ")).
			Icon(&aw.Icon{
				Value: icoFilePath,
				Type:  aw.IconTypeImage,
			}).
			Var("pickedRootItem", entry.id).
			Valid(true)

	}

	query := flag.Arg(0)
	if query != "" {
		wf.Filter(query)
	}
	wf.SendFeedback()

	return true
}

func handleUpdateCache(ctx *WorkflowExecCtx) bool {
	if flag.Arg(0) != "update-cache" {
		return false
	}

	fmt.Println("INFO", "begin update cache")
	ctx.imgCacheRepo.CacheImages()
	return true
}
func startBgCmd(wf *aw.Workflow) {
	programm, err := os.Executable()
	log.Println("INFO", "triggering BG img caching", programm)
	if err != nil {
		log.Println("ERR", "could not get executable", err.Error())
		return
	}

	err = wf.RunInBackground("CACHE_IMAGES", exec.Command(programm, "update-cache"))
	if err != nil {
		log.Println("ERR", "could not start BG process", err.Error())
	}

	log.Println("INFO", "BG image caching triggered")
}

func handleSetupDbPath(ctx *WorkflowExecCtx) bool {
	if ctx.SetupMode != SetupModeDbPath {
		return false
	}

	query := flag.Arg(0)
	if query == "" {
		wf.NewItem("Please enter the db file path").
			Valid(false)

		return sendFeedbackAndEnd()
	}

	home, _ := os.UserHomeDir()
	if home != "" {
		query = strings.ReplaceAll(query, "~", home)
	}
	// ~/Library/Containers/in.sinew.Enpass-Desktop/Data/Documents/Vaults/primary

	_, err := enpass.NewVault(query, logrus.TraceLevel)
	if err != nil {
		wf.NewItem(fmt.Sprintf("Invalid file: %s", err.Error())).
			Valid(false)

		return sendFeedbackAndEnd()
	}

	wf.NewItem("Accept path").
		Subtitle("↩ to accept").
		Var("enpassFile", query).
		Var("setupMode", SetupModeDbPassword).
		Valid(true)

	return sendFeedbackAndEnd()

}

func handleSetupDbPass(ctx *WorkflowExecCtx) bool {
	if ctx.SetupMode != SetupModeDbPassword {
		return false
	}

	query := flag.Arg(0)

	if query == "" {
		wf.NewItem("Please enter the db password").
			Valid(false)

		return sendFeedbackAndEnd()
	}

	log.Println("INFO", "open vault", ctx.EnpassFile)
	vault, err := enpass.NewVault(ctx.EnpassFile, logrus.TraceLevel)
	if err != nil {
		wf.NewItem(fmt.Sprintf("Invalid file: %s", err.Error())).
			Valid(false)

		return sendFeedbackAndEnd()
	}

	err = vault.Open(&enpass.VaultCredentials{
		Password: query,
	})
	if err != nil {
		log.Printf("could not open db %v\n", err)
		wf.NewItem("Could not unlock vault, invalid password?").
			Subtitle(fmt.Sprintf("ERR %s", err.Error())).
			Valid(false)

		return sendFeedbackAndEnd()
	}

	wf.NewItem("Accept password (will be stored in keychain)").
		Subtitle("↩ to accept").
		Var("enpassPass", query).
		Var("enpassFile", ctx.EnpassFile).
		Var("setupMode", SetupModeCommit).
		Valid(true)

	return sendFeedbackAndEnd()
}

func handleSetupComplete(ctx *WorkflowExecCtx) bool {
	if ctx.SetupMode != SetupModeCommit {
		return false
	}

	err := keyring.Set("alfred-enpass", "enpass", ctx.TmpEnpassPass)
	if err != nil {
		log.Println("ERR", "store data in keychain", err)
		return sendFeedbackAndEnd()
	}

	if err := wf.Config.Set("enpassFile", ctx.EnpassFile, false).Do(); err != nil {
		log.Println("ERR", "store data in keychain", err)
		return sendFeedbackAndEnd()
	}

	return sendFeedbackAndEnd()

}

func getEntries(ctx *WorkflowExecCtx) ([]*EnPassEntry, error) {
	vault := openAndAuthVault(ctx)
	if vault == nil {
		return []*EnPassEntry{}, errors.New("Error accessing vault")
	}

	cards, err := vault.GetEntries("", []string{})
	if err != nil {
		log.Println("ERR", "error vault.getEntries: %s", err.Error())
		return []*EnPassEntry{}, errors.New("Error accessing vault")
	}

	cards = lo.Filter(cards, func(card enpass.Card, i int) bool {
		return !card.IsDeleted() && !card.IsTrashed()
	})

	cardMap := lo.GroupBy(cards, func(t enpass.Card) string {
		return t.UUID
	})

	entries := make([]*EnPassEntry, 0)
	for k, secrets := range cardMap {
		first := secrets[0]
		entries = append(entries, &EnPassEntry{
			id:    k,
			cards: secrets,
			title: first.Title,
			ico:   first.Icon,
		})
	}

	sort.SliceStable(entries, func(i, j int) bool {
		return strings.ToLower(entries[i].title) < strings.ToLower(entries[j].title)
	})

	return entries, nil
}
