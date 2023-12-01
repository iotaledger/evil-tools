rm -f evil-tools
go build

./evil-tools accounts create --alias A

./evil-tools spammer --spammer blk -rate 1 --duration 10s --account A