rm -f evil-tools
rm -f *.dat
go build

./evil-tools accounts create --eviltools.accounts.create.alias A

./evil-tools spammer --eviltools.spammer.type blk --eviltools.spammer.rate 1 --eviltools.spammer.duration 10s --eviltools.spammer.account A

./evil-tools accounts destroy --eviltools.accounts.destroy.alias A

# this spam should not work, as account is now destroyed
./evil-tools spammer --eviltools.spammer.type blk --eviltools.spammer.rate 1 --eviltools.spammer.duration 10s --eviltools.spammer.account A
