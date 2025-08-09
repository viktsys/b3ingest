# B3 Trading Data Ingestion Tool

Uma aplicação CLI em Go para ingestão e análise de dados de negociações da B3, utilizando GORM como ORM e PostgreSQL como banco de dados.

## Arquitetura da Solução

### Componentes Principais

1. **CLI Interface** (Cobra): Interface de linha de comando com dois comandos principais
2. **Processador de Dados** (ingest/): Sistema de ingestão concorrente com workers
3. **API REST** (api/): Endpoint único para consulta de estatísticas agregadas
4. **Modelos de Dados** (models/): Estruturas GORM para trades e agregações diárias
5. **Banco de Dados** (database/): Configuração PostgreSQL com GORM

### Otimizações de Performance

- **Processamento Concorrente**: 128 workers processando batches de 2000 registros
- **Inserção em Lotes**: GORM batch inserts para reduzir latência do banco
- **Agregação SQL**: Cálculos agregados executados diretamente no PostgreSQL
- **Índices Estratégicos**: Índices compostos em (data_negocio, codigo_instrumento)
- **Connection Pool**: Configuração otimizada de pool de conexões

## Requisitos

- Go 1.22+
- Docker e Docker Compose
- PostgreSQL (via Docker)

## Estrutura do Projeto

```
├── cmd/                 # Comandos CLI (Cobra)
├── models/              # Modelos de dados (GORM)
├── database/            # Configuração do banco de dados
├── ingest/              # Processador de dados concorrente
├── api/                 # Handlers da API REST
├── data/                # Diretório para arquivos CSV
├── docker-compose.yml   # Orquestração de serviços
├── Dockerfile           # Container da aplicação
└── README.md            # Documentação
```

## Instalação e Configuração

### 1. Clonar e Preparar o Ambiente

```bash
git clone <repository-url>
cd b3ingest
```

### 2. Subir o ambiente usando Docker Compose
```
docker-compose up -d
```

### 3. Preparar os Dados

Baixe os arquivos CSV de negociações da B3 e coloque no diretório `data/`:

```bash
mkdir -p data
# Coloque seus arquivos .csv no diretório data/
```

**Estrutura esperada do CSV:**
- Separador: `;` (ponto e vírgula)
- Colunas: DataNegocio, CodigoInstrumento, PrecoNegocio, QuantidadeNegociada, HoraFechamento
- Formato da data: YYYY-MM-DD
- Formato do preço: decimal com vírgula (será convertido para ponto)

### 4. Compilar a Aplicação

```bash
go build -o bin/b3ingest .
```

### 5. Executar a aplicação no modo ingester

```bash
./bin/b3ingest ingest --data-dir data/
```

### 6. Consultar a API

#### Parâmetros

- `ticker` (obrigatório): Código do ativo (ex: "PETR4")
- `data_inicio` (opcional): Data inicial no formato ISO-8601 (YYYY-MM-DD)

#### Exemplos

```bash
# Consultar PETR4 nos últimos 7 dias
curl "http://localhost:8080/api/trades/stats?ticker=PETR4"

# Consultar VALE3 a partir de uma data específica
curl "http://localhost:8080/api/trades/stats?ticker=VALE3&data_inicio=2024-01-15"
```

#### Resposta

```json
{
  "ticker": "PETR4",
  "max_range_value": 20.50,
  "max_daily_volume": 150000
}
```

## Performance e Otimizações

### Ingestão de Dados

- **Concorrência**: 128 workers processando simultaneamente
- **Batch Size**: 2000 registros por lote
- **Memory Management**: Processamento streaming dos CSV
- **Transações**: Uso de transações para integridade dos dados

### Consultas

- **Agregação SQL**: Cálculos no banco de dados (carga maior, porém mais eficiente)
- **Índices Estratégicos**: Busca otimizada por ticker e data
- **Connection Pool**: Reutilização de conexões
- **Cache de Resultados**: Tabela de agregações pré-calculadas

### Estimativas de Performance

- **Ingestão**: ~50k-100k registros/minuto (dependendo do hardware)
- **Consultas**: <100ms para a maioria dos tickers
- **Memória**: ~50-100MB durante ingestão