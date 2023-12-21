rm -f evil-tools
rm -f *.dat
go build

./evil-tools accounts create --eviltools.accounts.create.alias A

./evil-tools spammer --spammer blk -rate 1 --duration 10s --account A