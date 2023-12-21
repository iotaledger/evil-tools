rm -f evil-tools
rm -f *.dat
go build

./evil-tools accounts create --eviltools.accounts.create.alias IMPLICIT --eviltools.accounts.create.implicit

./evil-tools spammer --spammer blk -rate 1 --duration 10s --account IMPLICIT