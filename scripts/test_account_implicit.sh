rm -f evil-tools
rm -f *.dat
go build

./evil-tools accounts create --alias IMPLICIT --implicit

./evil-tools spammer --spammer blk -rate 1 --duration 10s --account IMPLICIT