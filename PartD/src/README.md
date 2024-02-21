# 简介

Raft PartD 调试总结

## 第一关：实验测试

执行命令

```
go test -run PartD
go test -run PartD -race
VERBOSE=0 go test -run PartD -race | tee out.txt
```

## 第二关：并发测试

安装辅助测试插件

dslogs 和 dstest

脚本内容见 tools，在 mac 上：

```
sudo cp dslogs dstest /usr/local/bin
brew install python
pip3 install typer rich
```

执行 `dslogs -c 3 out.txt` 高亮分区查看日志（在 raft 目录下）
执行 `dstest PartD -p 30 -n 100` 进行并行测试

输入下面结果表示测试成功：

![](./resource/images/2024-02-21-23-20-06.png)

![](resource/images/2024-02-21-23-37-00.png)

![](resource/images/2024-02-21-23-53-07.png)
