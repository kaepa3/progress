package main

import (
	"bytes"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"sync"
	"text/template"
	"time"

	"github.com/BurntSushi/toml"
	log "github.com/cihub/seelog"
	jira "gopkg.in/andygrunwald/go-jira.v1"
)

var config Config

type Config struct {
	JiraURL string
	User    string
	Pass    string
	Jql     string
	Names   []string
	PostURL string
}

const (
	logConfig = `
<seelog type="adaptive" mininterval="200000000" maxinterval="1000000000" critmsgcount="5">
<formats>
    <format id="main" format="%Date(2006-01-02T15:04:05.999999999Z07:00) [%File:%FuncShort:%Line] [%LEV] %Msg%n" />
</formats>
<outputs formatid="main">
    <filter levels="trace,debug,info,warn,error,critical">
        <console />
    </filter>
    <filter levels="info,warn,error,critical">
        <rollingfile filename="log.log" type="size" maxsize="102400" maxrolls="1" />
    </filter>
</outputs>
</seelog>`
	target = 65
)

func CountToPersent(val int) string {
	return fmt.Sprintf("%.2f", float32(val)/target*100)
}

func initLogger() {
	logger, err := log.LoggerFromConfigAsBytes([]byte(logConfig))
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}
	log.ReplaceLogger(logger)
}

func main() {
	initLogger()
	toml.DecodeFile("./Config.toml", &config)
	HttpPost(editJson(getIssues()))
}

func getIssues() []Result {
	//	Jiraとの接続
	jiraClient, _ := jira.NewClient(nil, config.JiraURL)
	jiraClient.Authentication.SetBasicAuth(config.User, config.Pass)

	var wg sync.WaitGroup
	var retVal []Result
	for _, name := range config.Names {
		year := time.Now().Year()
		wg.Add(1)
		go func(jiraClient *jira.Client, name string, year int, wg *sync.WaitGroup) {
			retVal = append(retVal, CountData(jiraClient, name, year, wg))
			defer wg.Done()
		}(jiraClient, name, year, &wg)
	}
	wg.Wait()
	return retVal
}

func CountData(jiraClient *jira.Client, name string, year int, wg *sync.WaitGroup) Result {

	//　課題の取得
	retVal := Result{"", 0}
	opt := &jira.SearchOptions{MaxResults: 1000}
	jql := createJql(config.Jql, name, year)
	issues, _, err := jiraClient.Issue.Search(jql, opt)
	if err == nil {
		retVal.Name = name
		retVal.Count = len(issues)
	} else {
		log.Error(jql)
	}
	return retVal
}

type TemplateInfomation struct {
	Date    string
	Results []Result
}
type Result struct {
	Name  string
	Count int
}

func createJql(basejql string, name string, year int) string {
	return fmt.Sprintf(basejql, strconv.Itoa(year)+"/1/1", strconv.Itoa(year+1)+"/01/01", name)
}

func editJson(list []Result) string {
	log.Info(list)
	model := TemplateInfomation{time.Now().Format("2006/01/02"), list}
	text := `{
		"attachments":[
		   {
			  "fallback":"作業数",
			  "pretext":"{{.Date}}時点の作業数",
			  "color":"#D00000",
			  "fields":[
				{{range .Results}}{
				   "title":"{{.Name}}",
				   "value":"{{.Count}} issue/` + strconv.Itoa((target)) + ` {{.Count | CountToPersent}}%"
				},{{end}}
			  ]
		   }
		]
	}`

	f := template.FuncMap{
		"CountToPersent": CountToPersent,
	}
	tmpl, err := template.New("name").Funcs(f).Parse(text)
	if err != nil {
		log.Info(err)
		return `{"text":"Error"}`
	}
	buf := bytes.NewBuffer([]byte{})
	tmpl.Execute(buf, model)
	result := buf.String()
	log.Info(result)
	return result
}

func HttpPost(jsonStr string) error {

	req, err := http.NewRequest(
		"POST",
		config.PostURL,
		bytes.NewBuffer([]byte(jsonStr)),
	)
	if err != nil {
		return err
	}

	// Content-Type 設定
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return err
}
