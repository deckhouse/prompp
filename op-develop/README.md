# Как собирать, запускать и отлаживать

**Внимание!** Все команды выполняются из корня этого репозитория.

## Использование локальной библиотеки odarix-core-go

1. Расположите репозиторий [odarix-core](fox.flant.com/okmeter/odarix-core) в соседней папке.
2. В файле `go.mod` добавьте строчку
   ```
   replace github.com/odarix/odarix-core-go => ../odarix-core/go
   ```

## Сборка бинарей

1. Сборочный контейнер:
   ```
   docker build -t alpine_builder:3.20.3 -f op-develop/Dockerfile .
   ```
2. Далее работаем в нём:
   ```
   docker run -it --rm -v .:/src -v $(dirname $(pwd))/odarix-core:/odarix-core alpine_builder:3.20.3 /bin/bash
   ```
3. Первый раз необходимо выполнить полную сборку проекта:
   ```
   make build
   ```
4. В дальнейшем можно пересобирать исключительно go-файлы:
   ```
   make common-build
   ```

## Сборка контейнера для выката в тестовый кластер

1. Сборка образа (после сборки бинарей):
   ```
   docker build -t registry.okmetric.com/prometheus/prometheus:latest -f op.Dockerfile2 .
   ```
2. Далее пушим образ в реджистри:
   ```
   docker push registry.okmetric.com/prometheus/prometheus:latest
   ```
   если не пускает, уточнить данные для входа у @evgeny.bastrykov.
3. Подключиться к stage-кластеру:
   ```
   ssh okmeter.fsn-dev-kube-master01
   ```
4. Переключиться в рута:
   ```
   sudo su
   ```
4. Удалить под:
   ```
   kubectl -n d8-monitoring delete po -l prometheus=pp
   ```
   если модуль недавно перекатывали, то он может ссылаться на другой реджистри. Тогда надо пропатчить модуль:
   ```
   kubectl -n d8-monitoring patch prometheuses.monitoring.coreos.com pp --patch-file op-develop/module.patch.yaml --type=merge
   ```

## Воспроизведение запуска на директории с прода

1. Пусть у нас есть папка со следующими файлами/папками:
   ```
   <uuid - папка с head-ом>/
   <uuid - папка с head-ом>/
   <uuid - папка с head-ом>/
   client_id.uuid
   config.yaml
   head.log
   ```
2. Из этой папки запускаем собранный бинарник `prometheus` следующей командой:
   ```
   /path/to/prometheus --config.file=config.yaml --web.listen-address=":8080" --log.level=debug --storage.tsdb.path=.
   ```

## Отладка и запуск под gdb

1. Сборка odarix-core с отладочными символами (выполняется в корен проекта odarix-core):
   ```
   make compilation_mode=dbg build-entrypoint
   ```
2. Сборка prometheus с библиотекой с отладочными символами:
   - в файле `.promu.yml` в секцию `build.tags.all` добавить `dbg`;
   - собрать бинарь командой `make common-build`;
3. Запуск под gdb:
   ```
   cdir=/path/to/odarix-core gdb --args <строка запуска>
   ```

## Извлечение файлов из кластера (в случае работающего prom++)

1. Запускаем debug-контейнер:
   ```
   kubectl -n d8-monitoring debug prometheus-pp-0 -it --target=prometheus --share-processes --image=ubuntu
   ```
2. Файлы находятся по пути `/proc/1/root`:
   - рабочая директория `prometheus`;
   - конфиг `etc/prometheus/prometheus.yml`;
3. Вытащить файлы можно с помощью `kubectl cp`, используя в качестве имени пода дебаг контейнер (его назваение пишется на второй строке после запуска)

## Извлечение файлов из кластера (в случае неработающего prom++)

1. Файл конфига можно достать из секрета:
   ```
   kubectl -n d8-monitoring get secret prometheus-pp -o json | jq '.data["prometheus.yaml.gz"]' -r | base64 -d > db/config.yaml.gz
   ```
2. Доступ к файлам можно осуществить с помощью привилегерованного пода. Для этого нужно:
3. Выяснить на какой ноде находится под:
   ```
   kubectl -n d8-monitoring get pod prometheus-pp-debug-0 -o json | jq '.spec.nodeName' -r
   ```
4. Запускаем привилегированный под с селектором на этой ноде:

TODO: доделать извлечение файлов из неработающего прома