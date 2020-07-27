package main

import (
	"encoding/gob"
	"fmt"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/go-telegram-bot-api/telegram-bot-api"
	"github.com/joho/godotenv"
	"github.com/robfig/cron"
)

type telegramBot struct {
	bot               *tgbotapi.BotAPI
	updates           tgbotapi.UpdatesChannel
	messageID         int
	chatID            int64
	isAdmin           bool
	auth              string
	gobCacheFilePath  string
	adminID           int
	adminConversation *conversation
	remindList        []*conversation
	cron              *cron.Cron
}

type conversation struct {
	User *tgbotapi.User
	Chat *tgbotapi.Chat
}

type chacheConversation struct {
	AdminConversation *conversation
	RemindList        []*conversation
}

type chacheConversationHelper struct {
	AdminConversation conversation
	RemindList        []conversation
}

func (tel *telegramBot) getMessage() {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	tel.updates, _ = tel.bot.GetUpdatesChan(u)
	for update := range tel.updates {
		if update.Message == nil {
			// ignore any non-Message Updates
			continue
		}

		if update.Message.Text == "" {
			continue
		}

		tel.chatID = update.Message.Chat.ID
		tel.isAdmin = update.Message.From.ID == tel.adminID

		if update.Message.Chat.IsGroup() || update.Message.Chat.IsSuperGroup() {
			if update.Message.Entities != nil {
				for _, v := range *update.Message.Entities {
					if v.Type == "mention" {
						tel.switchFunc(update, v.Length+1)
					}
				}
			}
		} else {
			tel.switchFunc(update, 0)
		}

		tel.initMessageUserType()
	}
}

func (tel *telegramBot) setChatID(chatID int64) {
	tel.chatID = chatID
	return
}

func (tel *telegramBot) returnTextMessage(text string) {
	if tel.chatID == 0 {
		log.Panic("chatId为0, 无法发送消息")
		return
	}
	tel.sendTextMessage(text, tel.chatID, tel.messageID)
}

func (tel *telegramBot) sendTextMessage(text string, chatID int64, replyToMessageID int) {

	msg := tgbotapi.NewMessage(chatID, text)
	if replyToMessageID != 0 {
		msg.ReplyToMessageID = replyToMessageID
	}
	if _, err := tel.bot.Send(msg); err != nil {
		log.Panic(err)
	}
}

func (tel *telegramBot) initMessageUserType() {
	tel.messageID = 0
	tel.chatID = 0
	tel.isAdmin = false
}

func (tel *telegramBot) updateAdmin(user *tgbotapi.User, chat *tgbotapi.Chat) {
	tel.adminID = user.ID
	tel.adminConversation = &conversation{User: user, Chat: chat}
	tel.chache()
}

func (tel *telegramBot) GetAuth(l int) string {
	rand.Seed(time.Now().UnixNano())
	letters := []rune("0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")
	b := make([]rune, l)
	sl := len(letters)
	for i := range b {
		b[i] = letters[rand.Intn(sl)]
	}
	tel.auth = string(b)
	return tel.auth
}

func (tel *telegramBot) registerRemindList(con *conversation) bool {
	for _, v := range tel.remindList {
		if v.Chat.ID == con.Chat.ID {
			return false
		}
	}
	tel.remindList = append(tel.remindList, con)
	tel.chache()
	return true
}

func (tel *telegramBot) switchFunc(update tgbotapi.Update, lenght int) {
	messageText := ""
	if lenght > 0 {
		messageText = update.Message.Text[lenght:]
	} else {
		messageText = update.Message.Text
	}
	switch messageText {
	case tel.auth:
		tel.updateAdmin(update.Message.From, update.Message.Chat)
		tel.returnTextMessage("管理员注册成功")
		return
	case "close":
		if tel.isAdmin {
			tel.returnTextMessage("bot 程序已经关闭")
			os.Exit(0)
		} else {
			tel.return502()
			return
		}

	case "提臀提醒":
		if tel.registerRemindList(&conversation{User: update.Message.From, Chat: update.Message.Chat}) {
			tel.returnTextMessage("注册提臀提醒成功, 每隔十分钟会向您发送提臀提醒")
		} else {
			tel.returnTextMessage("这个会话已经注册过提臀提醒了")
		}
		return
	case "getRemindList":
		lenght := len(tel.remindList)
		text := ""
		if lenght == 0 {
			text = "当前提臀队列为空, 没有待提醒"
		} else {
			time := tel.cron.Entries()[0].Next
			text = "当前队列有" + strconv.Itoa(lenght) + "个用户将于" + time.Format("2006/01/02 15:04:05") + "开始提醒提臀\n分别为: \n"
			for k, v := range tel.remindList {
				if v.Chat.IsSuperGroup() || v.Chat.IsGroup() {
					text += v.Chat.Title
				} else {
					text += v.Chat.UserName
				}
				if k < (lenght - 1) {
					text += "\n"
				}
			}
		}
		tel.returnTextMessage(text)
		return
	case "getAdminInfo":
		if tel.isAdmin {
			tel.returnTextMessage("当前管理员是: " + tel.adminConversation.User.UserName)
		} else {
			tel.return502()
		}
		return
	default:
		tel.returnTextMessage(update.Message.Text)
		return
	}
}

func (tel *telegramBot) return502() {
	tel.returnTextMessage("您没有权限进行这项操作")
}

func (tel *telegramBot) registerCron(spec string, cmd func()) {

	tel.cron.AddFunc(spec, cmd)
	log.Println("任务开始, 请管理员输入", tel.GetAuth(64))

	go tel.cron.Start()
	defer tel.cron.Stop()
}

func (tel *telegramBot) chache() {
	cache := chacheConversation{AdminConversation: tel.adminConversation, RemindList: tel.remindList}

	makePath(tel.gobCacheFilePath, 0777)
	file, err := os.OpenFile(tel.gobCacheFilePath, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer file.Close()
	enc := gob.NewEncoder(file)
	enc.Encode(cache)
}

func (tel *telegramBot) readCache() {
	makePath(tel.gobCacheFilePath, 0777)
	File, err := os.OpenFile(tel.gobCacheFilePath, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer File.Close()

	M := chacheConversationHelper{}
	D := gob.NewDecoder(File)
	if D.Decode(&M) != nil {
		return
	}

	tmpList := make([]*conversation, 0)
	for _, v := range M.RemindList {
		tmpList = append(tmpList, &v)
	}

	//还原admin信息
	tel.adminID = M.AdminConversation.User.ID
	tel.adminConversation = &conversation{User: M.AdminConversation.User, Chat: M.AdminConversation.Chat}
	//还原提醒信息
	tel.remindList = tmpList
}

func newTelegramBot(apiToken string) *telegramBot {
	bot, err := tgbotapi.NewBotAPI(apiToken)
	if err != nil {
		log.Panic(err)
	}
	// bot.Debug = true
	log.Printf("Authorized on account %s", bot.Self.UserName)
	tel := &telegramBot{bot: bot}
	//准备定时任务
	tel.cron = cron.New()
	tel.initMessageUserType()

	return tel
}

func makePath(path string, perm os.FileMode) (bool, error) {
	_, err := os.Stat(path)
	if err == nil || os.IsExist(err) {
		return true, nil
	}
	if os.IsNotExist(err) {
		dir := filepath.Dir(path)
		os.MkdirAll(dir, perm)
		return true, nil
	}
	return false, err
}

func main() {
	if err := godotenv.Load(".env"); err != nil {
		log.Fatalln(err)
		os.Exit(1)
	}

	logFilePath := os.Getenv("LOG_OUTPUT_FILE_PATH")

	if logFilePath == "" {
		logFilePath = "./.tmp/out_put.log"
	}

	makePath(logFilePath, 0777)
	logFile, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		fmt.Println("open log file failed, err:", err)
		return
	}
	log.SetOutput(logFile)

	log.Println("bot start!!!")
	apiToken := os.Getenv("TELEGRAM_APITOKEN")

	tel := newTelegramBot(apiToken)

	tmpCachePath := os.Getenv("GOB_CACHE_FILE_PATH")
	if tmpCachePath == "" {
		tmpCachePath = "./.tmp/cache.gob"
	}
	tel.gobCacheFilePath = tmpCachePath
	tel.readCache()

	i := 0
	text := ""
	var cstSh, _ = time.LoadLocation("Asia/Shanghai") //上海
	tel.registerCron("0 */10 * * *", func() {
		now := time.Now()
		Hour := now.Hour()
		if Hour > 9 && Hour < 18 {

			if len(tel.remindList) > 0 {
				log.Println("今天第", i, "次消息通知")
				text = "现在时间" + now.In(cstSh).Format("2006/01/02 15:04:05") + "\n 提臀小助手提醒您: 请注意提臀, 不要久坐, 保护屁股, 人人有责"
				for _, v := range tel.remindList {
					tel.sendTextMessage(text, v.Chat.ID, 0)
				}
				i++
			} else {
				log.Println("提醒队列未注册")
			}
		}
	})

	tel.getMessage()
}
