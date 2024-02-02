go build


./evil-tools accounts create --accounts.create.alias A
./evil-tools accounts info accounts

./evil-tools accounts create --accounts.create.alias B --accounts.create.implicit --accounts.create.transition

./evil-tools accounts create --accounts.create.alias C

./evil-tools accounts delegate --accounts.delegate.fromAlias A

./evil-tools accounts delegate --accounts.delegate.fromAlias X
./evil-tools accounts delegate --accounts.delegate.fromAlias X
./evil-tools accounts delegate --accounts.delegate.fromAlias X

./evil-tools accounts info accounts

./evil-tools accounts info delegation
#
