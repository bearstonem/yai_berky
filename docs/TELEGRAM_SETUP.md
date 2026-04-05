# Telegram Integration Setup Guide

This guide will help you set up Telegram integration so you can chat with your Helm agent through Telegram.

## Prerequisites

1. **A Telegram Bot Token** - You need to create a bot via BotFather to get this token.

## Step 1: Create a Telegram Bot

1. Open Telegram and search for **@BotFather**
2. Send `/newbot` to create a new bot
3. Follow the prompts to choose a name and username for your bot
4. BotFather will give you a token that looks like: `123456789:ABCdefGhIJKlmNoPQRsTUVwxyZ`

## Step 2: Set the Environment Variable

Add your bot token to your shell environment:

```bash
export TELEGRAM_BOT_TOKEN="your_token_here"
```

To make this permanent, add it to your `~/.bashrc` or `~/.zshrc`:

```bash
echo 'export TELEGRAM_BOT_TOKEN="your_token_here"' >> ~/.bashrc
source ~/.bashrc
```

## Step 3: Test Your Bot

Once you have the skill and token set up, test that everything works:

```bash
# Verify the bot token is working
skill_telegram_bot '{"command": "get_me"}'
```

You should see your bot's information including the bot name and username.

## How It Works

The `skill_telegram_bot` skill provides these commands:

- `get_me` - Verify your bot token and get bot info
- `send_message` - Send a message to a Telegram chat
- `get_updates` - Poll for new messages (long polling)
- `set_webhook` - Set up webhook for instant message delivery
- `get_webhook_info` - Check current webhook configuration
- `delete_webhook` - Remove webhook
- `answer_callback_query` - Answer inline button callbacks

## Chatting with the Agent via Telegram

To enable Telegram chatting, the agent needs to:

1. **Poll for messages** using `get_updates`
2. **Extract new messages** from the update response
3. **Process with the AI engine** to generate a response
4. **Send back the response** using `send_message`

### Message Flow

```
Telegram User → Bot → get_updates → Agent Engine → send_message → Bot → User
```

## Important Notes

1. **Users must contact the bot first** - Telegram bots can only message users who have initiated contact
2. **Parse mode** - Use "Markdown" or "HTML" in the send_message command
3. **Rate limits** - Telegram has message rate limits; don't spam the API
4. **Update offset** - Use the update_id from the last processed message + 1 to avoid duplicate processing

## Getting Help

If you need to see the available commands:

```bash
skill_telegram_bot '{"command": "help"}'
```
