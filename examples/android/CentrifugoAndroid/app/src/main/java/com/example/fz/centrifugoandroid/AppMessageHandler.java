package com.example.fz.centrifugoandroid;

import android.app.Activity;
import android.content.Context;
import android.widget.TextView;

import centrifuge.Message;
import centrifuge.MessageHandler;
import centrifuge.Sub;

public class AppMessageHandler implements MessageHandler {
    protected MainActivity context;

    public AppMessageHandler(Context context) {
        this.context = (MainActivity) context;
    }

    @Override
    public void onMessage(Sub sub, final Message message) throws Exception {
        context.runOnUiThread(new Runnable() {
            @Override
            public void run() {
                TextView tv = (TextView) ((Activity) context).findViewById(R.id.text);
                tv.setText(message.getData());
            }
        });
    }
}
