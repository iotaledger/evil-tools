# evil-tools

## Spammer

```bash
go build
./evil-tools [spammer,accounts] [[script-flag-set]]
```

### `spammer`
Usage of script flag set:
- `account`:  _string_ - Account alias to be used for the spam. Account should be created first with accounts tool.
- `dbc`: _duration_ - delayBetweenConflicts - Time delay between conflicts in double spend spamming
- `deep`: _bool_ - Enable the deep spam, by reusing outputs created during the spam. To enable provide an empty flag.
- `duration`: _string_ - Spam duration. If not provided spam will lats infinitely. Format: separated by commas list of decimal numbers, each with optional fraction and a unit suffix, such as '300ms', '-1.5h' or '2h45m'.
Valid time units are 'ns', 'us', 'ms', 's', 'm', 'h'.
- `rate`: _int_ - Spamming rate for provided 'spammer'. Format: numbers separated with comma, e.g. 10,100,1 if three spammers were provided for 'spammer' parameter. (default 1)
- `scenario`: _string_ - Name of the EvilBatch that should be used for the spam. By default, uses Scenario1. Possible scenarios can be found in evilwallet/customscenarion.go.
- `spammer`: _string_ - Spammers used during test. Format: strings separated with comma, available options: 'blk' - block, 'tx' - transaction, 'ds' - double spends spammers, 'nds' - n-spends spammer, 'custom' - spams with provided scenario, 'bb' - blowball
- `unit`: _duration_ - Time unit for the spamming rate. Format: decimal numbers, each with optional fraction and a unit suffix, such as '300ms', '-1.5h' or '2h45m'.
Valid time units are 'ns', 'us', 'ms', 's', 'm', 'h'. (default 1s)
- `urls`: _string_ - API urls for clients used in test separated with commas

Possible spam scenarios:
`blk, tx, peace, bb, ds, conflict-circle, guava ,orange, mango, pear, lemon, banana, kiwi`


### `accounts`
Usage for accounts [COMMAND] [FLAGS], multiple commands can be chained together.

COMMAND: **create**
- `alias`: _string_ - the alias name of new created account
- `implicit` - create an implicit account
- `noBIF` - create account without Block Issuer Feature, can only be set false no if implicit is false, as each account created implicitly needs to have BIF.
- `noTransition` - account should not be transitioned to a full account if created with implicit address. Transition enabled by default, to disable provide an empty flag.

COMMAND: **convert**
- `alias`: _string_ - the implicit account to be converted to full account with BIF.

COMMAND: **destroy**
- `alias`: _string_ - the alias name of the account to be destroyed
- `expirySlot`: _int_ - the expiry slot of the account to be destroyed


### Examples for the spammer
Possible
Spam with scenario `tx`
```bash
./evil-tools spammer -spammer custom -scenario tx -rate 10 -duration 100s
```
Rare spam with rate in BPM instead of BPS
```bash
./evil-tools spammer -spammer custom -scenario tx -unit 1m -rate 1 -duration 100s
```
Infinite spam is enabled when no duration flag is provided.
```bash
./evil-tools spammer -spammer custom -scenario tx -unit 1m -rate 1
```
You can provide urls for clients:
```bash
./evil-tools spammer -urls "http://localhost:8050,http://localhost:8060" -spammer custom -scenario tx -rate 1
```
Enable deep spam:
```bash
./evil-tools spammer -spammer custom -scenario tx -unit 1m -rate 1 -deep
```

### Examples for the accounts
Create implicit account with alias `A`:
```bash
./evil-tools accounts create -alias A -implicit
```
Create account with genesis account paying for creation transaction:
```bash
./evil-tools accounts create -alias A
```

### Scenario diagrams:
##### No conflicts
- `single-tx`

![Single transaction](./img/evil-scenario-tx.png "Single transaction")

- `peace`

![Peace](./img/evil-scenario-peace.png "Peace")

- `bb` - blow ball structure

##### Conflicts
- `ds`

![Double spend](./img/evil-scenario-ds.png "Double spend")

- `conflict-circle`

![Conflict circle](./img/evil-scenario-conflict-circle.png "Conflict circle")

- `guava`

![Guava](./img/evil-scenario-guava.png "Guava")

- `orange`

![Orange](./img/evil-scenario-orange.png "Orange")

- `mango`

![Mango](./img/evil-scenario-mango.png "Mango")

- `pear`

![Pear](./img/evil-scenario-pear.png "Pear")

- `lemon`

![Lemon](./img/evil-scenario-lemon.png "Lemon")

- `banana`

![Banana](./img/evil-scenario-banana.png "Banana")

- `kiwi`

![Kiwi](./img/evil-scenario-kiwi.png "Kiwi")


