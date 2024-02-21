# 简介

Raft PartB 调试总结

## 第一关：实验测试

执行命令

```
go test -run PartC

VERBOSE=0 go test -run PartC | tee out.txt
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
执行 `dstest PartB -p 30 -n 100` 进行并行测试

输入下面结果表示测试成功：

![](/resources/images/2024-02-22-01-42-07.png)

![](/resources/images/2024-02-22-01-54-55.png)