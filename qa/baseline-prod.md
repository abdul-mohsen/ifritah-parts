# Harsh HK probe results

Base URL: $BaseUrl  
At: 2026-08-15 11:30:39Z  
Total: 42 | Pass: 26 | Fail: 14 | Error: 2

| id | category | verdict | http | results | first | ms | description |
| --- | --- | --- | ---: | ---: | --- | ---: | --- |
| vin-01 | vin | PASS | 200 | 0 |  | 7732 | Hyundai Tucson 2016 (golden) |
| vin-02 | vin | PASS | 200 | 0 |  | 5720 | Kia Sportage 2018 (golden) |
| vin-03 | vin | PASS | 200 | 0 |  | 5978 | Hyundai Elantra 2017 |
| vin-04 | vin | PASS | 200 | 0 |  | 6135 | Kia K5 2021 |
| vin-05 | vin | PASS | 200 | 0 |  | 5657 | Hyundai Palisade 2022 |
| vin-99 | vin | FAIL_ROUTING | 200 | 0 |  | 848 | BUG: GET /api/vin/:vin falls through to SPA |
| oem-golden-01 | oem | FAIL_RANK | 200 | 20 | F 026 407 124 | 44208 | Golden: 26300-35505 oil filter |
| oem-golden-02 | oem | FAIL_MISSING | 200 | 20 | CU 23 019 | 38369 | Golden: 97133-D3000 cabin filter |
| oem-golden-03 | oem | FAIL_MISSING | 200 | 2 | 97133 | 18433 | Golden article-number prefix 97133 |
| oem-golden-04 | oem | FAIL_MISSING | 200 | 20 | LIFE-TIME-FILTER | 16079 | Golden category text: cabin air filter |
| oem-real-46321-3B650 | oem | FAIL_MISSING | 200 | 8 | SG 1700 | 29024 | Real HK OEM: Hyundai auto-trans mount [46321-3B650] |
| oem-real-54528-4A100 | oem | FAIL_MISSING | 200 | 8 | 54528 | 56405 | Real HK OEM: Kia lower ball joint [54528-4A100] |
| oem-real-55700-3S000 | oem | FAIL_JUNK_DESC | 200 | 1 | 557003S000 | 32678 | Real HK OEM: Hyundai Sonata rear axle beam [55700-3S000] |
| oem-real-92101-3S050 | oem | PASS_ANY | 200 | 1 | 921013S050 | 32835 | Real HK OEM: Hyundai Sonata headlight [92101-3S050] |
| oem-real-25100-25000 | oem | FAIL_MISSING | 200 | 10 | FWP2200 | 28568 | Real HK OEM: Hyundai water pump [25100-25000] |
| oem-real-58101-3SA00 | oem | FAIL_MISSING | 200 | 14 | 958.0 | 30462 | Real HK OEM: Hyundai Sonata front brake pad [58101-3SA00] |
| oem-real-51712-2S000 | oem | FAIL_JUNK_DESC | 200 | 1 | 517122S000 | 30404 | Real HK OEM: Hyundai Tucson strut [51712-2S000] |
| oem-real-55311-2S000 | oem | FAIL_MISSING | 200 | 20 | MM-KI053 | 41460 | Real HK OEM: Hyundai Tucson rear coil spring [55311-2S000] |
| text-oil%20filter | text | PASS_ANY | 200 | 20 | A52-9618 | 22176 | Text: 'oil filter' |
| text-brake%20pad | text | PASS_ANY | 200 | 20 | 1754Q | 24532 | Text: 'brake pad' |
| text-brake%20disc | text | PASS_ANY | 200 | 20 | ADS8 | 21027 | Text: 'brake disc' |
| text-air%20filter | text | PASS_ANY | 200 | 20 | A63475 | 20192 | Text: 'air filter' |
| text-spark%20plug | text | PASS_ANY | 200 | 20 | HY-305 | 14541 | Text: 'spark plug' |
| text-wiper%20blade | text | PASS_ANY | 200 | 20 | ASH7-0001 | 19342 | Text: 'wiper blade' |
| text-headlight | text | PASS_ANY | 200 | 20 | 39042 | 18765 | Text: 'headlight' |
| text-tail%20light | text | PASS_ANY | 200 | 20 | 39003 | 14274 | Text: 'tail light' |
| text-shock%20absorber | text | PASS_ANY | 200 | 20 | HYAB-CM10SAR-KIT | 14569 | Text: 'shock absorber' |
| text-ball%20joint | text | PASS_ANY | 200 | 20 | SBK-8142 | 14711 | Text: 'ball joint' |
| veh-01 | vehicle | FAIL_MISSING | 200 | 20 | LIFE-TIME-FILTER | 12294 | Tucson 2016: cabin air filter |
| veh-02 | vehicle | PASS_ANY | 200 | 20 | A52-9618 | 16740 | Tucson 2016: oil filter |
| veh-03 | vehicle | PASS_ANY | 200 | 20 | 1754Q | 19791 | Tucson 2016: brake pad |
| veh-04 | vehicle | PASS_ANY | 200 | 20 | ASH7-0001 | 11975 | Tucson 2016: wiper |
| rec-01 | recall | PASS | 200 | 1 |  | 369 | HYUNDAI TUCSON 2016 recalls |
| rec-02 | recall | PASS | 200 | 1 |  | 394 | KIA SPORTAGE 2018 recalls |
| rec-03 | recall | PASS | 200 | 1 |  | 833 | HYUNDAI SANTA FE 2019 recalls |
| rec-04 | recall | PASS | 200 | 1 |  | 763 | KIA SORENTO 2021 recalls |
| cat-01 | catalog | PASS | 200 | 2 |  | 412 | Models list |
| cat-02 | catalog | ERROR | 500 | 0 |  | 908 | Vehicles for HYUNDAI TUCSON 2016 |
| cat-03 | catalog | FAIL_EMPTY | 200 | 0 |  | 1260 | Groups for vehicle 10001 |
| boundary-01 | boundary | ERROR | 0 | 0 |  | 90024 | Toyota 90915-YZZE1 (should be zero/warning) |
| det-01 | detail | PASS_LOADED | 200 | 0 |  | 1108 | Detail: legacyArticleId 100001 (oil filter) |
| det-02 | detail | PASS_LOADED | 200 | 0 |  | 1140 | Detail: legacyArticleId 100307 (cabin filter golden) |

