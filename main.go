package main

//planinfo。正式名称"Plan & Info"(プランアンドインフォ)
//TEDの邦題「先延ばし魔の頭の中はどうなっているのか」内のアイディア「ライフカレンダー」をターミナルで再現し、
//更にニュース等を受け取れるようにしたもの。
//機能//
//RSSを複数個指定し、取得。画面下部にてスクロールさせる
//目標日付と目標名を設定し、現在から残り何日かを（□）で表現

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"math"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"time"
	"unicode/utf8"

	"io/ioutil"

	"github.com/mattn/go-runewidth"
	"github.com/mitchellh/go-homedir"
	"github.com/nsf/termbox-go"
)

//Setting アプリケーションの設定
type Setting struct {
	UnderInfoScrollSpeed int      `json:"UnderInfoScrollSpeed"` //msごとに下部のスクロール速度
	FlushRate            int      `json:"FlushRate"`            //画面更新間隔(ms)
	UseRSS               bool     `json:"UseRSS"`               //RSSを使うか？
	RSSURL               []string `json:"RSSURL"`               //RSS取得のためのURL(空白の場合はNHKニュース)
	PlanBoxAmount        int      `json:"PlanBoxAmount"`        //プランボックス表示数(-1の場合は自動設定)
	MaxPlanBoxAmount     int      `json:"MaxPlanBoxAmount"`     //プランボックスの最大数(-1の場合は無制限)
	DebugFlag            bool     `json:"DebugFlag"`            //詳細表示フラグ
	AutoPlanBoxAmount    bool     `json:"AutoPlanBoxAmount"`    //プランボックス自動調節フラグ（この場合PlanBoxAmountは書き換えられる）
	MyPlan               Plan     `json:"MyPlan"`               //予定を設定する
}

//NewSetting Setting構造体を新規作成する。
func NewSetting() Setting {
	return Setting{}
}

//Plan 目標日付と名前を持つ。また日付にNowと書くと起動日時が書かれる。（その後、設定ファイルは更新される。）
type Plan struct {
	Name       string `json:"Name"`       //名前
	StartDate  string `Json:"StartDate"`  //開始日時
	TargetDate string `json:"TargetDate"` //目標日時

}

//NewPlan 計画の目標日時と開始日時、そして名前を設定する。
func NewPlan(n, start, end string) Plan {
	return Plan{n, start, end}
}

//PlanData Planによく似ているが、内部データをtime.Time型で保持する
type PlanData struct {
	Name       string    //名前
	StartDate  time.Time //開始日時
	TargetDate time.Time //目標日時
}

//NewPlanData 新規作成する。引数はPlan型で内部で変換されて処理される。
func NewPlanData(p *Plan) PlanData {

	name, start, target := p.Name, p.StartDate, p.TargetDate
	var sd, ed time.Time
	if start == "Now" {
		start = time.Now().Format(timeLayoutDate)
		p.StartDate = start //上書き
	}
	if ct, err := time.Parse(timeLayoutDate, start); err != nil {
		//変換失敗
		sd = time.Date(2000, 1, 2, 0, 0, 0, 0, time.Local)
		p.StartDate = sd.Format(timeLayoutDate) //上書き
	} else {
		sd = ct
	}
	if ct, err := time.Parse(timeLayoutDate, target); err != nil {
		//変換失敗
		ed = time.Date(2000, 1, 2, 0, 0, 0, 0, time.Local)
		p.TargetDate = ed.Format(timeLayoutDate) //上書き
	} else {
		ed = ct
	}
	return PlanData{
		Name:       name,
		StartDate:  sd,
		TargetDate: ed,
	}
}

//RssData RSSから取得したデータを抜き出す
type RssData struct {
	Items []struct {
		Title       string `xml:"title"`
		Description string `xml:"description"`
	} `xml:"channel>item"`
}

var (
	home, _ = homedir.Dir()
	//SettingFilePath 設定ファイルの取得アドレス
	SettingFilePath = home + "/go-planinfo.json"
	defaultSetting  = Setting{
		//初期設定
		UnderInfoScrollSpeed: 50,
		FlushRate:            20,
		UseRSS:               true,
		RSSURL: []string{
			"http://www3.nhk.or.jp/rss/news/cat0.xml",
			"https://rss-weather.yahoo.co.jp/rss/days/4410.xml",
		},
		PlanBoxAmount:     100,
		MaxPlanBoxAmount:  1000,
		DebugFlag:         false,
		AutoPlanBoxAmount: true,
		MyPlan: Plan{
			Name:       "２０１８年まで",
			StartDate:  "2017/01/01",
			TargetDate: "2018/01/01",
		},
	}

	nowSetting     Setting  //アプリケーションの設定
	underInfo      []string //画面下部を流れる文字情報
	underInfoSI    = 0      //画面下部の文字の最左におけるStringIndex
	underInfoI     = 0      //Stringスライスのインデックス
	underInfoSIs   []int    //スライスの文字数
	width, height  int      //画面幅
	stonD, stoeD   int      //目標に対して、現在日時からの日数と開始日時からの日数
	lpn            int      //PlanBoxの現在の残量
	l1, l2, l3     string   //Planについての詳細表示のための行
	timeLayout     = "2006/01/02 15:04:05"
	timeLayoutDate = "2006/01/02"
	weekdayLayout  = [...]string{"日", "月", "火", "水", "木", "金", "土"}
	keyCh          = make(chan termbox.Key, 1) //なんのボタンが押されたかの送受信用
	timerCh        = make(chan bool)           //画面下部のスクロール用チャンネルフラグ
	fCh            = make(chan bool)           //画面更新用チャンネル
	nCh            = make(chan bool)           //ニュース取得用チャンネル
	dCh            = make(chan bool)           //日付変更時のチャンネルフラグ
	plans          []PlanData
	lplan          PlanData //現在表示中のPlan
)

func main() {
	//初期化とエラー処理
	fmt.Println("設定ファイルアドレス : " + SettingFilePath)
	Init()           //初期化
	Update()         //更新
	UpdatePlanInfo() //Planに関して計算する。
	go KeyEventLoop()
	go FlushTimer()
	go Today2TomorrowTimer()
	if nowSetting.UseRSS {
		//RSSが有効の時
		go UnderInfoScrollTimer()
		go GetNewsTimer()
	}
	mainLoop()
	defer termbox.Clear(termbox.ColorDefault, termbox.ColorDefault)
	defer termbox.Close()
}

//Init 初期化処理。表示する情報の取得を行う
func Init() {
	//画面サイズだけ取得して一旦終了させる。
	err := termbox.Init()
	if err != nil {
		panic(err)
	}
	width, height = termbox.Size()       //画面比取得
	termbox.Close()                      //一旦終了させる
	runtime.GOMAXPROCS(runtime.NumCPU()) //並列処理
	nowSetting = NewSetting()            //設定構造体作成
	//設定ファイルが存在するか調べる。
	_, err = os.Stat(SettingFilePath)
	if err == nil {
		//設定ファイルが存在する場合
		//設定ファイルから設定を取得。（設定ファイルが設定できなかった場合、デフォルト設定が適用される。
		fmt.Println("設定ファイルを読込中……")
		ls, ok, errmsg := LoadSettingFile()

		if ok == true {
			nowSetting = ls
		} else {
			//設定に不具合があった場合、エラーメッセージを5秒表示
			fmt.Println(errmsg)
			nowSetting = defaultSetting
		}
	} else {
		//設定ファイルが存在しない場合
		fmt.Println("初期設定ファイルを作成。初期設定で起動します。")
		SaveSettingFile(&defaultSetting)
		nowSetting = defaultSetting
	}

	//設定取得処理完了
	fmt.Println("設定を取得しました。起動中。")
	//横幅の設定の偶数奇数判定
	if float64(width%2) != 0 {
		//奇数
		width-- //偶数にする
	}
	if nowSetting.AutoPlanBoxAmount == true {
		//自動設定
		nowSetting.PlanBoxAmount = int(math.Floor(float64(width/2) * float64((height-5)/2))) //プランボックス表示領域。現在時刻、目標詳細、下部情報に領域を専有される
	}
	if nowSetting.MaxPlanBoxAmount <= nowSetting.PlanBoxAmount && nowSetting.MaxPlanBoxAmount != -1 {
		//最大数を設定
		nowSetting.PlanBoxAmount = nowSetting.MaxPlanBoxAmount
	}
	if len(nowSetting.RSSURL) == 0 && nowSetting.UseRSS {
		//空白の場合はRSSURLを自動設定
		nowSetting.RSSURL = defaultSetting.RSSURL
	}

	//Plan読み込み
	if nowSetting.MyPlan.Name == "" && nowSetting.MyPlan.StartDate == "" && nowSetting.MyPlan.TargetDate == "" {
		//Planの設定がなされていない場合はデフォルトを適用。
		nowSetting.MyPlan = defaultSetting.MyPlan
	}
	plans = []PlanData{NewPlanData(&nowSetting.MyPlan)} //PlanをPlanDataに変換
	lplan = plans[0]                                    //将来複数のPlanを見られるようにするための変数
	SaveSettingFile(&nowSetting)                        //自動設定の結果を上書き
	//すべての初期化が終わったので改めて立ち上げる
	if err = termbox.Init(); err != nil {
		panic(err)
	}
	termbox.Clear(termbox.ColorDefault, termbox.ColorDefault) //画面初期化
}

//Update RSSやその他データを取得し更新
func Update() {
	//表示文字列群
	underInfo = []string{}
	underInfoSIs = []int{}
	//冗長性確保
	lsspace := ""
	for i := 0; i < width; i++ {
		lsspace += " "
	}
	//表示文字列
	shs := ""
	//実行された時間。取得日時。
	gt := time.Now()
	shs = lsspace + "【time:" + gt.Format(timeLayout) + "】"
	AddUnderInfo(shs)
	//RSSデータを得ていく
	for _, url := range nowSetting.RSSURL {
		res, err := http.Get(url)

		if err != nil {
			shs = lsspace + "ERROR! URL:" + url
			AddUnderInfo(shs)
			continue
		}

		b, err := ioutil.ReadAll(res.Body)

		if err != nil {
			shs = lsspace + "ERROR! URL:" + url
			AddUnderInfo(shs)
			continue
		}

		rdata := new(RssData)
		err = xml.Unmarshal(b, &rdata)

		if err != nil {
			shs = lsspace + "ERROR! これはRSSではありません！ URL:" + url
			AddUnderInfo(shs)
			continue

		}

		for in, i := range rdata.Items {
			if in == 0 {
				shs = i.Title + " / " + i.Description
			} else {
				shs = lsspace + i.Title + " / " + i.Description
			}
			AddUnderInfo(shs)
		}

		shs = " (URL:" + url + ") "
		AddUnderInfo(shs)
	}
	AddUnderInfo(lsspace) //画面下部最後の余白。（これのおかげで最後のスクロールが流れ切っても、画面下部の一行は青く表示される。）
}

//ViewUpdate 画面上に文字列を表示する
func ViewUpdate() {
	nowtimeString := time.Now().Format(timeLayout)
	nowtimeString += " (" + weekdayLayout[time.Now().Weekday()] + ")"
	SetLine(0, nowtimeString, termbox.ColorDefault, termbox.ColorDefault)
	hi := 0 //文字列を置く位置を示す
	si := 0 //現在最左の文字はなん文字目？
ENDSETCELL:
	for in, s := range underInfo {
		if underInfoI <= in {
			//文字列最左以降
			for _, r := range s {
				if underInfoI < in || (underInfoI == in && underInfoSI <= si) {
					termbox.SetCell(hi, height-1, r, termbox.ColorDefault, termbox.ColorBlue)
					hi += runewidth.RuneWidth(r)
					if hi >= width {
						break ENDSETCELL
					}
				}
				si++
			}
			si = 0
		}
	}

}

//ScrollUnderInfo 文字列のスクロールをひとつ増やす
func ScrollUnderInfo() {
	underInfoSI++
	//文字数を取得
	if underInfoSI >= getInt(underInfoSIs, underInfoI) {
		underInfoSI = 0
		underInfoI++ //文字列スライスを一つ進める。
		if underInfoI >= len(underInfo)-1 {
			//文字列配列の最後尾の一つ前まで辿り着くとはじめから。（最後尾の余白は見せるため）
			underInfoI = 0
		}
	}
}

//UpdatePlanInfo Planのデータを更新する
func UpdatePlanInfo() {

	//Planデータの残り日数とかを計算する。
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	stoeD = int(lplan.TargetDate.Sub(lplan.StartDate).Hours()) / 24 //開始日から終了日までの総日数
	stonD = int(lplan.TargetDate.Sub(today).Hours() / 24)           //現在日時から終了日までの総日数
	if stoeD <= 0 {
		//0以下なら
		stoeD = 1 //強制的に一日を総日数とする。
	}
	if stonD < 0 {
		//0未満（負数）なら
		stonD = 0 //強制的に一日とする。
	}
	lpn = int(nowSetting.PlanBoxAmount * stonD / stoeD) //現在のPlanBoxの残量
	l1, l2, l3 = "", "", ""                             //詳細表示用の行を初期化
	//一行分の文字列も余白に追加
	l1 = lplan.Name + func(s string) (margin string) {
		margin = ""                                   ///余白文字列
		marginnum := width - runewidth.StringWidth(s) //余白の文字数を取得
		for i := 0; i < marginnum; i++ {
			margin += " " //余白を追加
		}

		return
	}(lplan.Name)

	//詳細表示
	if nowSetting.DebugFlag != true {
		//通常モード
		l2 += lplan.StartDate.Format(timeLayoutDate) + " から " + lplan.TargetDate.Format(timeLayoutDate) + " まで  現在:残り " + strconv.Itoa(stonD) + " 日"
		l2 += " / " + strconv.Itoa(stoeD) + "日 （" + strconv.Itoa(int(float64(stonD)/float64(stoeD)*100)) + "％ ）"
	}
}

//ViewPlan Planを描画
func ViewPlan() {

	//debug時のみ詳細表示をすぐさま更新する
	if nowSetting.DebugFlag == true {
		//debugモード
		temp, _ := exec.Command("vcgencmd", "measure_temp").Output() //温度を取得。（Raspbianのみ）
		l2, l3 = "", ""
		l2 += lplan.StartDate.Format(timeLayoutDate) + " から " + lplan.TargetDate.Format(timeLayoutDate) + " まで  現在:残り " + strconv.Itoa(stonD) + " 日"
		l2 += " / " + strconv.Itoa(stoeD) + "日（Screen:" + strconv.Itoa(height) + "x" + strconv.Itoa(width) + ")"
		l3 += strconv.Itoa(nowSetting.PlanBoxAmount) + "個/" + fmt.Sprint(float64(stonD)/float64(stoeD)*100) + "%"
		l3 += "CPU_Temptature:" + string(temp) + "/" + "1日あたり:" + fmt.Sprint(100/float64(stoeD)) + "%"
	}

	SetLine(1, l1, termbox.ColorWhite, termbox.ColorBlue)
	SetLine(2, l2, termbox.ColorDefault, termbox.ColorDefault)
	SetLine(3, l3, termbox.ColorDefault, termbox.ColorDefault)

	hi, wi := 4, 0
	for i := 0; i < nowSetting.PlanBoxAmount; i++ {
		if (nowSetting.PlanBoxAmount - lpn) > i {
			termbox.SetCell(wi, hi, '□', termbox.ColorDefault, termbox.ColorDefault)
		} else {
			termbox.SetCell(wi, hi, '■', termbox.ColorDefault, termbox.ColorDefault)
		}
		wi += 2
		if wi >= width {
			hi += 2
			if height-1 <= hi {
				break
			}
			wi = 0
		}
	}
}

//主要なループ
func mainLoop() {
	for {
		select {
		case key := <-keyCh:
			if key == termbox.KeyEsc || key == termbox.KeyCtrlC {
				return
			}
		case <-timerCh:
			ScrollUnderInfo()
		case <-nCh:
			Update()
		case <-fCh:
			termbox.Clear(termbox.ColorDefault, termbox.ColorDefault)
			ViewUpdate()
			ViewPlan()
			termbox.Flush()
		case <-dCh:
			//日付が変わるとき
			UpdatePlanInfo()
		}
	}
}

//SetLine h=表示する高さ s=表示文字列 f=文字表示色 b=背景色
func SetLine(h int, s string, f, b termbox.Attribute) {
	hi := 0 //文字列を置く位置を示す
	for _, r := range s {
		termbox.SetCell(hi, h, r, f, b)
		hi += runewidth.RuneWidth(r)
	}
}

//GetStringNum Stringの文字数を取得
func GetStringNum(s string) (num int) {
	num = utf8.RuneCountInString(s)
	return
}

//AddUnderInfo 画面下部の文字列情報を追加する
func AddUnderInfo(s string) {
	underInfo = append(underInfo, s)
	underInfoSIs = append(underInfoSIs, GetStringNum(s))
}

//KeyEventLoop キー押下を検出するためのループ関数
func KeyEventLoop() {
	for {
		ev := termbox.PollEvent()
		switch ev.Type {
		case termbox.EventKey:
			keyCh <- ev.Key
		}
	}
}

//UnderInfoScrollTimer 時間をカウントし、文字をスクロールさせる
func UnderInfoScrollTimer() {
	for {
		var leftrune rune
		leftrune = ' '

		//画面下部のスクロール文字列の中で最左のRuneを取得し、そのByte数で速度を割ることで見かけの速度を一定にする。
		for i, r := range getString(underInfo, underInfoI) {
			if i == underInfoSI {
				leftrune = r
				break
			}
		}
		time.Sleep(time.Duration(nowSetting.UnderInfoScrollSpeed*runewidth.RuneWidth(leftrune)) * time.Millisecond)
		timerCh <- true
	}
}

//FlushTimer 画面更新間隔
func FlushTimer() {
	for {
		time.Sleep(time.Duration(nowSetting.FlushRate) * time.Millisecond)
		fCh <- true
	}
}

//GetNewsTimer 1時間毎に発生させるフラグ。ニュース取得用
func GetNewsTimer() {
	for {
		//特定の時間まで待機。そしてチャンネルを送信。
		now := time.Now()
		//nowhour := now.Hour() //現在時刻
		var gettime time.Time
		/*switch {
		case nowhour >= 21 && nowhour < 24 && nowhour != 0: //0時(24時)に取得する場合
			nextday := now.AddDate(0, 0, 1)
			gettime = time.Date(nextday.Year(), nextday.Month(), nextday.Day(), 0, 0, 0, 0, time.Local)
		case nowhour >= 0 || nowhour == 24 && nowhour < 3:
			gettime = time.Date(now.Year(), now.Month(), now.Day(), 3, 0, 0, 0, time.Local)

		case nowhour >= 3 && nowhour < 6:
			gettime = time.Date(now.Year(), now.Month(), now.Day(), 6, 0, 0, 0, time.Local)

		case nowhour >= 6 && nowhour < 9:
			gettime = time.Date(now.Year(), now.Month(), now.Day(), 9, 0, 0, 0, time.Local)

		case nowhour >= 9 && nowhour < 12:
			gettime = time.Date(now.Year(), now.Month(), now.Day(), 12, 0, 0, 0, time.Local)

		case nowhour >= 12 && nowhour < 15:
			gettime = time.Date(now.Year(), now.Month(), now.Day(), 15, 0, 0, 0, time.Local)

		case nowhour >= 15 && nowhour < 18:
			gettime = time.Date(now.Year(), now.Month(), now.Day(), 18, 0, 0, 0, time.Local)

		case nowhour >= 18 && nowhour < 21:
			gettime = time.Date(now.Year(), now.Month(), now.Day(), 21, 0, 0, 0, time.Local)
		default:
			//例外はとりあえず一時間後に取得
			gettime = now.Add(time.Duration(1) * time.Hour)
		}*/

		ohl := now.Add(time.Duration(1) * time.Hour) //一時間後を取得
		gettime = time.Date(ohl.Year(), ohl.Month(), ohl.Day(), ohl.Hour(), 0, 0, 0, time.Local)
		sub := gettime.Sub(now) //指定の時刻からの差分
		time.Sleep(sub)         //待機
		nCh <- true             //チャンネル送信

		//画面下部情報のスクロール位置を初期化。
		underInfoI = 0
		underInfoSI = 0
	}
}

//Today2TomorrowTimer 日付が変わった時にチャンネルを送信する。
func Today2TomorrowTimer() {
	for {
		tott := time.Now().AddDate(0, 0, 1)                                                  //tomorrow this time 明日のこの時間
		tomorrow := time.Date(tott.Year(), tott.Month(), tott.Day(), 0, 0, 0, 0, time.Local) //明日の午前0時
		time.Sleep(tomorrow.Sub(time.Now()))
		//僅かな誤差があった時のために一応日付を確認する。
		for {
			if time.Now().Day() == tott.Day() {
				break
			}
			time.Sleep(time.Duration(1) * time.Second)
		}
		dCh <- true
	}
}

//SaveSettingFile 設定ファイルを保存する。　引数は保存する構造体。
func SaveSettingFile(sset *Setting) {

	var settingfile *os.File
	var err error

	if _, err = os.Stat(SettingFilePath); err == nil {
		//存在する場合
		settingfile, err = os.OpenFile(SettingFilePath, os.O_WRONLY, 0666)
	} else {
		//存在しない場合
		settingfile, err = os.Create(SettingFilePath)
	}

	if err != nil {
		//エラーが発生した場合
		fmt.Println("設定ファイル取得/作成エラー！")
		fmt.Println(err)
	}

	defer settingfile.Close()

	encoder := json.NewEncoder(settingfile)

	err = encoder.Encode(sset)

	if err != nil {
		//エラーが発生した場合
		fmt.Println("ERROR! JSON保存処理エラー！")
		fmt.Println(err)
	}
}

//LoadSettingFile 設定ファイルをロードする。 戻り値は成否
func LoadSettingFile() (loadSetting Setting, ok bool, errmsg string) {

	loadSetting = Setting{}
	errmsg = "" //エラー時のメッセージ
	ok = true   //構造体がうまく読み込めたのかのフラグ
	if _, err := os.Stat(SettingFilePath); err != nil {
		//存在しない場合
		return Setting{}, false, "ERROR! ファイルが存在しません！"
	}

	settingfile, err := os.OpenFile(SettingFilePath, os.O_RDONLY, 0666)
	if err != nil {
		fmt.Println(err)
		return Setting{}, false, "ERROR! ファイルが開けません！"
	}
	defer settingfile.Close()

	decoder := json.NewDecoder(settingfile)
	err = decoder.Decode(&loadSetting)

	if err != nil {
		fmt.Println(err)
		return Setting{}, false, "ERROR! 設定JSONを読み込めませんでした!"
	}

	//設定が正しくなされているか確認
	switch {
	case loadSetting.FlushRate <= 0:
		errmsg = "ERROR! FlushRateは1以上の値を設定してください。"
		ok = false

	case loadSetting.MaxPlanBoxAmount < -1 || loadSetting.MaxPlanBoxAmount == 0:
		errmsg = "ERROR! MaxPlanBoxは-1ならば無制限。　もしくは1以上の値を設定してください。"
		ok = false

	case loadSetting.PlanBoxAmount < 0:
		errmsg = "ERROR! PlanBoxは0以上の値を設定してください。"
		ok = false

	case loadSetting.UnderInfoScrollSpeed <= 0:
		errmsg = "ERROR! UnderInfoのScrollSpeedは1以上の値を設定してください。"
		ok = false

	}
	return

}

func getString(strArr []string, index int) string {
	//String配列をインデックス外を取得しそうになったらスペース一つのStiringを返す。
	if index <= len(strArr)-1 && index >= 0 {
		return strArr[index]
	}
	return " "
}

func getInt(intArr []int, index int) int {
	//String配列をインデックス外を取得しそうになったら0を返す。
	if index <= len(intArr)-1 && index >= 0 {
		return intArr[index]
	}
	return 0
}
