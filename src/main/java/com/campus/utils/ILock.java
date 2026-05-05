package com.campus.utils;

public interface ILock {
    /**
     * 尝试获取锁
     * @param timeoutSec
     * @return
     */
     boolean tryLock(long timeoutSec);

    /**
     * 释放锁
     */
    void delLock();
}
