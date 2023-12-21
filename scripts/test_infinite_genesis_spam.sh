rm -f evil-tools
rm -f *.dat
go build

./evil-tools spammer --eviltools.spammer.type --eviltools.spammer.scenario guava --eviltools.spammer.rate 20