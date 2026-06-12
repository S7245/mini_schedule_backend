```

  /btw 要能查到运行日志 log

    # 实时跟踪 brand 日志
    tail -f logs/brand.log

    # 实时跟踪 admin 日志
    tail -f logs/admin.log


  /btw 要能查到运行日志 log

    查看日志的方法

    # 实时跟踪 brand 日志
    tail -f logs/brand.log

    # 实时跟踪 admin 日志
    tail -f logs/admin.log

  passing. Next step is to test it by running ./run.sh, if you want. (disable
  recaps in /config)

  /btw 要能查到运行日志 log

    # 实时跟踪 brand 日志
    tail -f logs/brand.log

    # 实时跟踪 admin 日志
    tail -f logs/admin.log

    # 同时跟踪两个服务的日志
  /btw 要能查到运行日志 log

                                                           # 实时跟踪 brand 日志                                                             tail -f logs/brand.log
                                                            # 实时跟踪 admin 日志                                                             tail -f logs/admin.log
                                                            # 同时跟踪两个服务的日志
    tail -f logs/brand.log logs/admin.log
                                             # 查看最近 100 行                                                                 tail -n 100 logs/admin.log
                                                        # 搜索错误                                                                        grep -i error logs/brand.log
                                                      对应的进程信息
                                                                    PID 文件在 backend/run/                                                           目录下（brand.pid、admin.pid），可以用来确认进程是否还在运行：
    cat run/brand.pid          # 查看 PID
    kill -0 $(cat run/brand.pid) && echo "运行中"

    如果你希望脚本启动后自动提示日志路径或加一个 ./run.sh logs 子命令直接 tail
    日志，可以告诉主进程帮你补上。
```



```
wire_gen.go 是手改的，建议你 go generate ./... 重新生成确认一致
```