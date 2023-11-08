# evil-tools

## Spammer

```bash
go build
./evil-tools [spammer,accounts] [[script-flag-set]]
```

### `spammer`
Usage of script flag set:
- account string - Account alias to be used for the spam. Account should be created first with accounts tool.
- dbc duration - delayBetweenConflicts - Time delay between conflicts in double spend spamming
- deep - Enable the deep spam, by reusing outputs created during the spam.
- duration string - 
Spam duration. If not provided spam will lats infinitely. Format: separated by commas list of decimal numbers, each with optional fraction and a unit suffix, such as '300ms', '-1.5h' or '2h45m'.
Valid time units are 'ns', 'us', 'ms', 's', 'm', 'h'.
- rate int - Spamming rate for provided 'spammer'. Format: numbers separated with comma, e.g. 10,100,1 if three spammers were provided for 'spammer' parameter. (default 1)
- scenario string - Name of the EvilBatch that should be used for the spam. By default uses Scenario1. Possible scenarios can be found in evilwallet/customscenarion.go.
- spammer string - Spammers used during test. Format: strings separated with comma, available options: 'blk' - block, 'tx' - transaction, 'ds' - double spends spammers, 'nds' - n-spends spammer, 'custom' - spams with provided scenario, 'bb' - blowball
- unit duration - Time unit for the spamming rate. Format: decimal numbers, each with optional fraction and a unit suffix, such as '300ms', '-1.5h' or '2h45m'.
Valid time units are 'ns', 'us', 'ms', 's', 'm', 'h'. (default 1s)
- urls string - API urls for clients used in test separated with commas

Possible spam scenarios:
`blk, tx, peace, bb, ds, conflict-circle, guava ,orange, mango, pear, lemon, banana, kiwi`



### Examples
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


