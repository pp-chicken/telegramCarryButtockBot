package main

import (
	"fmt"
	"log"
	"math/rand"
	"os"
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
	adminID           int
	adminConversation *conversation
	remindList        []*conversation
	cron              *cron.Cron
}

type conversation struct {
	user *tgbotapi.User
	chat *tgbotapi.Chat
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
			fmt.Println(update.Message)
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
	tel.adminConversation = &conversation{user: user, chat: chat}
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
		if v.chat.ID == con.chat.ID {
			return false
		}
	}
	tel.remindList = append(tel.remindList, con)
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
		if tel.registerRemindList(&conversation{user: update.Message.From, chat: update.Message.Chat}) {
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
				if v.chat.IsSuperGroup() || v.chat.IsGroup() {
					text += v.chat.Title
				} else {
					text += v.chat.UserName
				}
				if k < (lenght - 1) {
					text += "\n"
				}
			}
		}
		tel.returnTextMessage(text)
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

func run() {
	if err := godotenv.Load(".env"); err != nil {
		log.Fatalln(err)
		os.Exit(1)
	}

	fmt.Println("bot start!!!")
	apiToken := os.Getenv("TELEGRAM_APITOKEN")

	tel := newTelegramBot(apiToken)

	i := 0
	text := ""
	tel.registerCron("0 */10 * * *", func() {
		if tel.adminConversation.user.ID != 0 {
			log.Println("第", i, "次消息通知")
			text = "现在时间" + time.Now().Format("2006/01/02 15:04:05") + "\n 提臀小助手提醒您: 请注意提臀, 不要久坐, 保护屁股, 人人有责"
			for _, v := range tel.remindList {
				tel.sendTextMessage(text, v.chat.ID, 0)
			}
			i++
		} else {
			log.Println("提醒队列未注册")
		}
	})

	tel.getMessage()
}

func main() {
	run()
}
