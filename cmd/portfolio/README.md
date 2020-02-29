## Простой плагин для мониторинга портфолио в меню-баре для [BitBar](https://getbitbar.com/)

### Установка
```
mkdir -p ~/.bitbar/lib
go build -o ~/.bitbar/lib/tinkoff-portfolio
echo 'API_KEY=YOUR_API_KEY ~/.bitbar/lib/tinkoff-portfolio' >~/.bitbar/tinkoff-portfolio.15s.sh
chmod +x ~/.bitbar/tinkoff-portfolio.15s.sh
```
