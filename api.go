package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	_ "github.com/Go-SQL-Driver/MySQL"
	"github.com/gorilla/mux"
	"github.com/russross/blackfriday"
	"html"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
)

type Topic struct {
	Tid  int    `json:"tid"`
	Name string `json:"name"`
}

type User struct {
	uid    int
	PubUid int    `json:"pub_uid"`
	Name   string `json:"name"`
	Avatar string `json:"avatar"`
	Points int    `json:"points"`
}

func (self User) getUid() int {
	return self.uid
}

type Answer struct {
	Aid      int    `json:"aid"`
	Content  string `json:"content"`
	Creator  *User  `json:"creator"`
	Votes    int    `json:"votes"`
	Comments int    `json:"comments"`
	Created  string `json:"created"`
}

type Question struct {
	Qid      int      `json:"qid"`
	Title    string   `json:"title"`
	Content  string   `json:"content"`
	Votes    int      `json:"votes"`
	Creator  *User    `json:"creator"`
	Answers  int      `json:"answers"`
	Comments int      `json:"comments"`
	Created  int      `json:"created"`
	Topics   []*Topic `json:"topics"`
}

func listQuestions(qtype string, sort string) []*Question {
	var qsts []*Question
	if qtype == "all" {
		qsts = getQuestionItems(sort)
	}
	if len(qsts) == 0 {
		return qsts
	}
	// -------------------------------------------------------------------------
	// topic, user 数据处理
	// -------------------------------------------------------------------------
	tids := make(map[int]int)
	var uids []int
	for _, qst := range qsts {
		for _, topic := range qst.Topics {
			tids[topic.Tid] = topic.Tid
		}
		uids = append(uids, qst.Creator.getUid())
	}
	var keys []int
	for k, _ := range tids {
		keys = append(keys, k)
	}
	//查询话题信息
	var topics = getTopicNameByID(keys)
	//获取用户信息
	var users = getUserByID(uids)
	// -------------------------------------------------------------------------
	// 数据整合
	// -------------------------------------------------------------------------
	for _, qst := range qsts {
		for _, topic := range qst.Topics {
			topic.Name = topics[topic.Tid]
		}
		qst.Creator = users[qst.Creator.getUid()]
	}

	return qsts
}

//logic methods
func getQuestionItems(sort string) []*Question {
	var questions []*Question
	var orderby string
	switch sort {
	case "newest":
		orderby = "create_time"
	case "vote":
		orderby = "vote_score"
	case "active":
		orderby = "last_edit_time"
	default:
		orderby = ""
	}

	if orderby == "" {
		return questions
	}

	db := NewDB()
	sql := fmt.Sprintf("select a.qid, b.rev_text, a.creator, a.create_time, a.answers, (a.vote_up_num - a.vote_down_num) as vote_num from questions as a left join question_revisions as b on a.qid = b.qid and a.rev_id = b.rev_num  where a.is_closed=0 and a.is_deleted=0 order by a.%s desc limit 0, 40", orderby)
	rows, err := db.Query(sql)
	checkErr(err)
	defer rows.Close()

	var (
		qid      int
		rev_text string
		created  int
		answers  int
		votes    int
		uid      int
	)

	for rows.Next() {
		err := rows.Scan(&qid, &rev_text, &uid, &created, &answers, &votes)
		checkErr(err)
		//decode question rev text
		arr := parseQuestionRevText(rev_text)

		//topic obj
		aTids := splitStr2Int(arr["topic"])
		var topics []*Topic
		for _, tid := range aTids {
			topics = append(topics, &Topic{Tid: tid})
		}

		//user obj
		creator := &User{uid: uid}

		questions = append(questions, &Question{Qid: qid, Title: arr["title"], Content: arr["content"], Creator: creator, Created: created, Answers: answers, Votes: votes, Topics: topics})
	}

	return questions
}

func getTopicNameByID(aTids []int) map[int]string {
	topics := make(map[int]string)

	if len(aTids) == 0 {
		return topics
	}

	sTids := joinInt2Str(aTids)

	db := NewDB()
	sql := fmt.Sprintf("select a.tid, b.rev_text from topics as a left join topic_revisions as b on a.tid=b.tid and a.rev_id=b.rev_num where a.tid in (%s)", sTids)
	rows, err := db.Query(sql)
	checkErr(err)
	var (
		tid      int
		rev_text string
	)
	for rows.Next() {
		err := rows.Scan(&tid, &rev_text)
		checkErr(err)
		//decode topic rev text
		topics[tid] = parseTopicRevText(rev_text)
	}
	return topics
}

func splitStr2Int(str string) []int {
	var arr []int
	a := strings.Split(str, ",")
	for _, sTid := range a {
		iTid, _ := strconv.Atoi(sTid)
		arr = append(arr, iTid)
	}
	return arr
}

func joinInt2Str(aTids []int) string {
	var arr []string
	for _, tid := range aTids {
		sTid := strconv.Itoa(tid)
		arr = append(arr, sTid)
	}
	return strings.Join(arr, ",")
}

func getUserByID(uids []int) map[int]*User {
	users := make(map[int]*User)
	if len(uids) == 0 {
		return users
	}
	sUids := joinInt2Str(uids)
	db := NewDB()
	sql := fmt.Sprintf("select uid, pub_uid, name, avatar, points from users where uid in (%s)", sUids)
	rows, err := db.Query(sql)
	checkErr(err)
	var (
		uid     int
		pub_uid int
		name    string
		avatar  string
		points  int
	)
	for rows.Next() {
		err := rows.Scan(&uid, &pub_uid, &name, &avatar, &points)
		checkErr(err)
		users[uid] = &User{uid: uid, PubUid: pub_uid, Name: name, Avatar: avatar, Points: points}
	}
	return users
}

func parseQuestionRevText(str string) map[string]string {
	arr := make(map[string]string)
	re := regexp.MustCompile(`<topic>([\S\s]+?)</topic>\s*?<title>([\S\s]+?)</title>\s*?<content>([\S\s]*?)</content>`)
	matches := re.FindStringSubmatch(str)
	arr["topic"] = matches[1]
	arr["title"] = matches[2]
	arr["content"] = matches[3]
	if len(arr["content"]) > 0 {
		//<coding-2 lang="py">
		//</coding>
		//arr["content"] = string(blackfriday.MarkdownCommon([]byte(matches[3])))
		str := html.UnescapeString(arr["content"])
		//reg := regexp.MustCompile(`<coding[\S\s]+?>`)
		//s.Replace("foo", "o", "0", -1)

		//log.Println(str)

		str = regexp.MustCompile(`&lt;coding-\d+\s+?lang="[a-z|A-Z]+"&gt;`).ReplaceAllString(str, "```")
		str = regexp.MustCompile(`&lt;/coding&gt;`).ReplaceAllString(str, "```")
		arr["content"] = string(blackfriday.MarkdownCommon([]byte(str)))
	}
	return arr
}

func parseTopicRevText(str string) string {
	re := regexp.MustCompile(`<title>([\S\s]+?)</title>`)
	matches := re.FindStringSubmatch(str)
	return matches[1]
}

//helper
func checkErr(err error) {
	if err != nil {
		log.Fatal("err: ", err)
	}
}

func NewDB() *sql.DB {
	db, err := sql.Open("mysql", "admin:1qaz2wsx@tcp(192.168.2.130:3306)/dewen?charset=utf8")
	checkErr(err)
	return db
}

func main() {

	r := mux.NewRouter()
	r.HandleFunc("/questions/all/{sort:[a-z]+}", QuestionsHandler)                  // /questions/all/newest,vote,active
	r.HandleFunc("/questions/unanswered/{sort:[a-z]+}", QuestionsUnansweredHandler) // /questions/unanswered/newest,vote
	r.HandleFunc("/questions/hot/{time:[a-z]+}", QuestionsHotHandler)               // /questions/hot/recent,week,month

	http.Handle("/", r)

	log.Println("Listening...")
	http.ListenAndServe(":3000", nil)
}

//控制器
func QuestionsHandler(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)
	sort := params["sort"]

	var questions []*Question = listQuestions("all", sort)

	js, err := json.Marshal(questions)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(js)

	//w.Write([]byte("111"))
}

func QuestionsUnansweredHandler(w http.ResponseWriter, r *http.Request) {
}

func QuestionsHotHandler(w http.ResponseWriter, r *http.Request) {
}
