# 概要

- 膨大な量の画像データをAWSのS3へアーカイブするための処理

# 使い方

## コマンドライン引数一覧

| 引数名 | 概要 | 備考 |
|---------|---------|---------|
|-bucket|バケット名||
|-local|アーカイブ対象のディレクトリ||
|-region|AWSのリージョン名|デフォルトでap-northeast-1|
|-cred|クレデンシャルファイルのパス|指定しない場合は~/.aws/credentialsに記載したクレデンシャルが使用される|
|-archive|アーカイブされたファイルのパスが記載されるjsonファイルのパス|デフォルトでは実行された箇所の直下にarchivesというディレクトリを作成し、その直下に対象となるファイルのパスをベースにした名前が割り振られて作成される|
|-storage-class|S3へ保存する際のプランを指定|デフォルトではGLACIER 長期保存のアーカイブを前提としているため|


## 使い方の例

```
 go run main.go -bucket {バケット名}　-local {アーカイブ対象ディレクトリ} 
```