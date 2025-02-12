# Релиз новой версии

1. Тэг релиза формируется как склейка тэга исходной версии прома и версии наших изменений, например, v2.53.2-0.1.0. Предварительно надо проверить, что такой тэг свободен в том числе в [модуле](https://fox.flant.com/deckhouse/observability/prompp-module/-/tags).
2. В этом репозитории создаём тэг релиза. В комментарии желательно перечислить изменения. Дожидаемся завершения пайплайна, привязанного к этому тэгу.
3. В [репозитории модуля](https://fox.flant.com/deckhouse/observability/prompp-module) в файлах `images/prompp(tools)/werf.inc.yaml` прописываем тэг релиза. В файле `CHANGELOG.md` в самый верх добавляем описание релиза, разделяя изменения на секции:
   - Fixes
   - Features and improvements
   - Breaking changes
   - Other
4. Коммитим изменения и вешаем тэг релиза на репозиторий модуля.
5. Пишем сообщение в [канал deckhouse-releases](https://loop.flant.ru/flant/channels/deckhouse-releases) по шаблону:
   ```
   ### :prometheus: [Prom++](https://fox.flant.com/deckhouse/observability/prometheus-plus-plus) тэг релиза

   Каналы обновлений: **Все** (либо конкретные Alpha, Beta, EarlyAccess и так далее)
   Редакции: **FE**

   ##### Копия описания из CHANGELOG.md с заголовками 5 уровня
   ```
6. Возвращаемся в репу модуля, в пайпплайн от тэга релиза. Тыкаем сборку и выкладку в нужные ветки. После выкладки в комментариях пишем: Выложено.
