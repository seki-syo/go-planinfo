# go-planinfo
 Manage events and information on the terminal  
ターミナルで予定と情報を管理しよう！  
元ネタはTEDのティム・アーバン氏の「先延ばし魔の頭の中はどうなっているのか」のライフカレンダー。  
Golangの練習も兼ねているので、コードが洗練されていないのは堪忍していただきたい。  
RSSを設定して流すこともできるよ！  
# 使い方
`go get github.com/seki-syo/go-planinfo`  
`go-planinfo`  
設定はホームディレクトリのgo-planinfo.jsonにて設定する。（初回起動時にデフォルト設定で新規作成します。）
## 設定項目
### UnderInfoScrollSpeed
画面下部を流れるRSS情報のスクロールスピード(ms)
### FlushRate
画面更新間隔(ms)
### RSSURL
取得するRSSのURL(配列で指定します。)。一時間ごとに更新します。
### PlanBoxAmount
表示するボックスの数。後述のAutoPlanBoxAmountがtrueの場合は無視されます。
### MaxPlanBoxAmount
表示するボックスの最大数。-1にすると無制限になります。
### DebugFlag
デバッグフラグ。trueにすると、詳細情報が閲覧できますが、負荷が上昇します。
### AutoPlanBoxAmount
trueの場合、画面サイズ（起動時）に最適な数のPlanBoxを自動設定します。
### MyPlan
==========
予定を設定します。
現状は一つのみです。
#### Name
予定の名前です。
#### StartDate
開始日を指定します。（例:2017/01/01）
#### TargetDate
終了日を指定します。（例:2017/12/31）
