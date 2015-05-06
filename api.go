package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	_ "github.com/Go-SQL-Driver/MySQL"
	"github.com/gorilla/mux"
	//"github.com/russross/blackfriday"
	"github.com/shurcooL/go/github_flavored_markdown"
	"html"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	//"time"
	"bytes"
	"html/template"
	//"os"
)

const (
	USER        = 1
	TOPIC       = 2
	QUESTION    = 3
	ANSWER      = 4
	COMMENT     = 6
	COMMENT_QST = 7
	COMMENT_ANS = 8
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
	Aid          int        `json:"aid"`
	Content      string     `json:"content"`
	Creator      *User      `json:"creator"`
	Votes        int        `json:"votes"`
	Comments     int        `json:"comments"`
	Created      int        `json:"created"`
	CommentItems []*Comment `json:"commentItems"`
}

type Question struct {
	Qid          int        `json:"qid"`
	Title        string     `json:"title"`
	Content      string     `json:"content"`
	Votes        int        `json:"votes"`
	Creator      *User      `json:"creator"`
	Answers      int        `json:"answers"`
	Comments     int        `json:"comments"`
	Created      int        `json:"created"`
	Topics       []*Topic   `json:"topics"`
	CommentItems []*Comment `json:"commentItems"`
	AnswerItems  []*Answer  `json:"answerItems"`
}

type Comment struct {
	Cid     int    `json:"cid"`
	Creator *User  `json:"creator"`
	Content string `json:content`
	Created int    `json:"created"`
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
	defer rows.Close()
	checkErr(err)

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
	defer rows.Close()
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

func getTopicByID(aTids []int) []*Topic {
	var topics []*Topic

	if len(aTids) == 0 {
		return topics
	}

	sTids := joinInt2Str(aTids)

	db := NewDB()
	sql := fmt.Sprintf("select a.tid, b.rev_text from topics as a left join topic_revisions as b on a.tid=b.tid and a.rev_id=b.rev_num where a.tid in (%s)", sTids)
	rows, err := db.Query(sql)
	defer rows.Close()
	checkErr(err)
	var (
		tid      int
		rev_text string
	)
	for rows.Next() {
		err := rows.Scan(&tid, &rev_text)
		checkErr(err)
		//decode topic rev text
		topics = append(topics, &Topic{Tid: tid, Name: parseTopicRevText(rev_text)})
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
	defer rows.Close()
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

func getQuestionByQid(id int) Question {
	var question Question
	var (
		qid          int
		rev_text     string
		created      int
		answers      int
		votes        int
		uid          int
		comments_num int
	)
	db := NewDB()
	err := db.QueryRow("select a.qid, b.rev_text, a.creator, a.create_time, a.answers, (a.vote_up_num - a.vote_down_num) as vote_num, a.comments_num from questions as a left join question_revisions as b on a.qid = b.qid and a.rev_id = b.rev_num  where a.qid=? and a.is_closed=0 and a.is_deleted=0", id).Scan(&qid, &rev_text, &uid, &created, &answers, &votes, &comments_num)
	checkErr(err)

	//decode question rev text
	arr := parseQuestionRevText(rev_text)

	//topic obj
	aTids := splitStr2Int(arr["topic"])
	topics := getTopicByID(aTids)

	//user obj
	uids := []int{uid}
	users := getUserByID(uids)
	creator := users[uid]

	question = Question{Qid: qid, Title: arr["title"], Content: arr["content"], Creator: creator, Created: created, Answers: answers, Votes: votes, Topics: topics, Comments: comments_num}
	return question
}

//获取实体评论
func getCommentsByObjType(pid int, cmt_type int) []*Comment {
	var comments []*Comment
	db := NewDB()
	sql := fmt.Sprintf("select a.cid, a.content, a.create_time, b.uid,b.pub_uid,b.name,b.avatar,b.points from comments as a left join users as b on a.creator = b.uid where pid=%d and type=%d and is_deleted=0 order by a.cid asc", pid, cmt_type)
	rows, err := db.Query(sql)
	defer rows.Close()
	checkErr(err)
	var (
		cid         int
		content     string
		create_time int
		uid         int
		pub_uid     int
		name        string
		avatar      string
		points      int
	)

	for rows.Next() {
		err := rows.Scan(&cid, &content, &create_time, &uid, &pub_uid, &name, &avatar, &points)
		checkErr(err)

		creator := &User{uid: uid, PubUid: pub_uid, Name: name, Avatar: avatar, Points: points}
		comments = append(comments, &Comment{Cid: cid, Creator: creator, Content: content, Created: create_time})
	}
	return comments
}

//获取实体评论
func getAnswersByQid(qid int) []*Answer {
	var answers []*Answer
	db := NewDB()
	sql := fmt.Sprintf("select a.aid, a.creator, a.create_time, (a.vote_up_num-a.vote_down_num) as vote_num,a.comments_num, b.rev_text from answers as a left join answer_revisions as b on a.aid = b.aid and a.rev_id=b.rev_num where qid=%d and a.is_deleted=0 order by a.aid asc", qid)
	rows, err := db.Query(sql)
	defer rows.Close()
	checkErr(err)
	var (
		aid          int
		uid          int
		create_time  int
		vote_num     int
		comments_num int
		rev_text     string
	)
	for rows.Next() {
		err := rows.Scan(&aid, &uid, &create_time, &vote_num, &comments_num, &rev_text)
		checkErr(err)

		creator := &User{uid: uid}
		cnt := parseAnswerRevText(rev_text)
		answers = append(answers, &Answer{Aid: aid, Creator: creator, Content: cnt, Created: create_time, Comments: comments_num, Votes: vote_num})
	}

	var uids []int
	for _, ans := range answers {
		uids = append(uids, ans.Creator.getUid())
	}
	//获取用户信息
	var users = getUserByID(uids)
	//整合
	for _, ans := range answers {
		ans.Creator = users[ans.Creator.getUid()]
		if ans.Comments > 0 {
			ans.CommentItems = getCommentsByObjType(ans.Aid, COMMENT_ANS)
		}
	}

	return answers
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
		str = regexp.MustCompile(`\r+`).ReplaceAllString(str, "\n")
		str = regexp.MustCompile(`\n+`).ReplaceAllString(str, "\n")
		//str = regexp.MustCompile("``").ReplaceAllString(str, "```")
		str = regexp.MustCompile(`\s*&lt;coding-[\S\s]+?&gt;\s*`).ReplaceAllString(str, "\n\r```")
		str = regexp.MustCompile(`\s*&lt;/coding&gt;`).ReplaceAllString(str, "```\n\r")

		str = html.UnescapeString(string(github_flavored_markdown.Markdown([]byte(str))))
		//str = string(blackfriday.MarkdownCommon([]byte(str)))
		//log.Println(str)
		//str = html.UnescapeString(str)
		arr["content"] = str
	}
	return arr
}

func parseTopicRevText(str string) string {
	re := regexp.MustCompile(`<title>([\S\s]+?)</title>`)
	matches := re.FindStringSubmatch(str)
	return matches[1]
}

func parseAnswerRevText(str string) string {
	re := regexp.MustCompile(`<content>([\S\s]+?)</content>`)
	matches := re.FindStringSubmatch(str)
	text := matches[1]
	if len(text) > 0 {
		//<coding-2 lang="py">
		//</coding>
		//arr["content"] = string(blackfriday.MarkdownCommon([]byte(matches[3])))
		str := html.UnescapeString(text)
		//reg := regexp.MustCompile(`<coding[\S\s]+?>`)
		//s.Replace("foo", "o", "0", -1)

		//log.Println(str)
		str = regexp.MustCompile(`\r+`).ReplaceAllString(str, "\n")
		str = regexp.MustCompile(`\n+`).ReplaceAllString(str, "\n")
		//str = regexp.MustCompile("``").ReplaceAllString(str, "```")
		str = regexp.MustCompile(`\s*&lt;coding-[\S\s]+?&gt;\s*`).ReplaceAllString(str, "\n\r```")
		str = regexp.MustCompile(`\s*&lt;/coding&gt;`).ReplaceAllString(str, "```\n\r")

		str = html.UnescapeString(string(github_flavored_markdown.Markdown([]byte(str))))
		//str = string(blackfriday.MarkdownCommon([]byte(str)))
		//log.Println(str)
		//str = html.UnescapeString(str)
		text = str
	}

	return text

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

func QuestionDetailHandler(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)
	qid := params["qid"]

	id, _ := strconv.Atoi(qid)
	if id == 0 {
		http.NotFound(w, r)
		return
	}

	qst := getQuestionByQid(id)
	if qst.Comments > 0 {
		qst.CommentItems = getCommentsByObjType(id, COMMENT_QST)
	}
	if qst.Answers > 0 {
		qst.AnswerItems = getAnswersByQid(id)
	}
	//log.Println(qst)

	t := template.New("qst")
	t, _ = t.Parse(`<div class="question">
            <div class="post-text article markdown-body">{{.Content}}</div>
            <div class="user-info owner">
                <div class="user-action-time">
                    提问于<span class="relativetime">{{.Created}}</span>
                </div>
                {{with .Creator}}
                <div class="user-avatar">
                    <a><img width="30" height="30" src="http://i.stack.imgur.com/FkjBe.png?s=30&g=1" /></a>
                </div>
                <div class="user-details">
                    <b>{{.Name}}</b><br>
                    <span class="reputation-points">{{.Points}}</span>
                    <span title="gold badges">
                        <span class="badge1">●</span>
                        <span class="badgecount">22</span>
                    </span>
                    <span title="silver badges">
                        <span class="badge2">●</span>
                        <span class="badgecount">22</span>
                    </span>
                    <span title="bronze badges">
                        <span class="badge3">●</span>
                        <span class="badgecount">22</span>
                    </span>
                </div>
                {{end}}
            </div>
            {{with .CommentItems}}
            <div class="comments">
                <table class="comments-table">
                    {{range .}}
                    <tr>
                        <td class="comment-text">{{.Content}} {{with .Creator}}-{{.Name}}{{end}}</td>
                    </tr>
                    {{end}}
                </table>
            </div>
            {{end}}
        </div>
        <div class="sort-wrap">
            <div class="subheader">{{.Answers}}个答案</div>
        </div>
        {{with .AnswerItems}}
        {{range .}}
        <div class="answer-summary">
            <div class="post-text article markdown-body">{{.Content}}</div>
            <div class="user-info">
                <div class="user-action-time">
                    提问于<span class="relativetime">{{.Created}}</span>
                </div>
                {{with .Creator}}
                <div class="user-avatar">
                    <a><img width="30" height="30" src="http://i.stack.imgur.com/FkjBe.png?s=30&g=1" /></a>
                </div>
                <div class="user-details">
                    <b>{{.Name}}</b><br>
                    <span class="reputation-points">{{.Points}}</span>
                    <span title="gold badges">
                        <span class="badge1">●</span>
                        <span class="badgecount">22</span>
                    </span>
                    <span title="silver badges">
                        <span class="badge2">●</span>
                        <span class="badgecount">22</span>
                    </span>
                    <span title="bronze badges">
                        <span class="badge3">●</span>
                        <span class="badgecount">22</span>
                    </span>
                </div>
                {{end}}
            </div>
            {{with .CommentItems}}
            <div class="comments">
                <table class="comments-table">
                    {{range .}}
                    <tr>
                        <td class="comment-text">{{.Content}} {{with .Creator}}-{{.Name}}{{end}}</td>
                    </tr>
                    {{end}}
                </table>
            </div>
            {{end}}
        </div>
        {{end}}
        {{end}}`)

	var doc bytes.Buffer
	t.Execute(&doc, qst)
	s := doc.String()
	//log.Println(s)

	type Data struct {
		Html string `json:"html"`
	}
	data := Data{Html: s}
	js, err := json.Marshal(data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(js)
}

func main() {

	r := mux.NewRouter()
	r.HandleFunc("/questions/all/{sort:[a-z]+}", QuestionsHandler)                  // /questions/all/newest,vote,active
	r.HandleFunc("/questions/unanswered/{sort:[a-z]+}", QuestionsUnansweredHandler) // /questions/unanswered/newest,vote
	r.HandleFunc("/questions/hot/{time:[a-z]+}", QuestionsHotHandler)               // /questions/hot/recent,week,month
	r.HandleFunc("/q/{qid:[0-9]+}", QuestionDetailHandler)

	http.Handle("/", r)

	log.Println("Listening...")
	http.ListenAndServe(":3000", nil)
}
