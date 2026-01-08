package com.example.clazz;

import java.io.Serializable;

// 目标：验证 IsAbstract, IsFinal, SuperClass, Annotations。
@Deprecated
@SuppressWarnings("unused")
public abstract class BaseClass implements Serializable {
}

final class FinalClass extends BaseClass implements Cloneable, Runnable {
    @Override public void run() {}
}