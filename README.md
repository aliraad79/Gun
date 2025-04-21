# Gun
modern high performance match engine


## Design Decisions
### Should process 500k orders per second
This means it should publish matches data in at most one second if the input order rate is about 500k messages


### No nned to restart
This module is not needed to be shut down and then power up everytime you need to add a orderbook and stop an existing one


## load test
```bash
cd loadTest
go run .
```
## topics
creating an Order -> NewOrder