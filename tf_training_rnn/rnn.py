import sys
import os
import json
import functools
import tensorflow as tf
from threading import Thread

class CurrFeeder(object):
    def __init__(self, train, file_path):
        print("Loading X, Y files from: %s" % file_path)
        self._batch_id = 0
        self._file_pos = 0
        self._file_line = 0
        self._train = train
        self._file_path = file_path
        self._file_size = os.path.getsize(file_path)
        print("File: %s Size: %d" % (file_path, self._file_size))

        with open(self._file_path, 'r') as f:
            line = f.readline()
            parts = line.split(':')
            self._line_len = len(json.loads(parts[0]))

    def get_seq_max_len(self):
        return self._line_len

    def next(self):
        return self._batch_data, self._batch_labels, self._batch_id

    def preload(self, batch_size):
        """ Return a batch of data. When dataset end is reached, start over.
        """
        print("Getting batch: %d, Line: %d, File Pos: %d" % (batch_size, self._file_line, self._file_pos))
        self._batch_data = []
        self._batch_labels = []
        with open(self._file_path, 'r') as f:
            if not self._train and self._file_pos == 0:
                f.seek(self._file_size * 0.75)
            else:
                f.seek(self._file_pos)

            no_end = False
            for line in iter(f.readline, ''):
                self._file_line += 1
                try:
                    parts = line.split(':')
                    # The line goes inverted so we are always have the most significant data at the beggining
                    self._batch_data.append(json.loads(parts[0]))
                    self._batch_labels.append(json.loads(parts[1]))
                    if len(self._batch_data) == batch_size:
                        no_end = True
                        break

                    if self._train and f.tell() > self._file_size * 0.75:
                        print("Out of training, restarting, Pos:", f.tell(), "Size:", self._file_size)
                        break

                except ValueError:
                    print("Problem parsing line:", ValueError)

            if no_end:
                self._file_pos = f.tell()
            else:
                print("EOF, starting again...")
                self._file_line = 0
                self._file_pos = 0
                self._batch_id = 0

        self._batch_id += 1

        print("Batch loaded...")


def lazy_property(function):
    attribute = '_' + function.__name__

    @property
    @functools.wraps(function)
    def wrapper(self):
        if not hasattr(self, attribute):
            setattr(self, attribute, function(self))
        return getattr(self, attribute)
    return wrapper


class SequenceClassification:
    def __init__(self, data, target, dropout, num_hidden=150, num_layers=2):
        self.data = data
        self.target = target
        self.dropout = dropout
        self._num_hidden = num_hidden
        self._num_layers = num_layers
        self.prediction
        self.accuracy
        self.optimize

    @lazy_property
    def prediction(self):
        # Recurrent network.
        network = tf.nn.rnn_cell.GRUCell(self._num_hidden)
        network = tf.nn.rnn_cell.DropoutWrapper(
            network, output_keep_prob=self.dropout)
        network = tf.nn.rnn_cell.MultiRNNCell([network] * self._num_layers)
        output, _ = tf.nn.dynamic_rnn(network, data, dtype=tf.float32)
        # Select last output.
        output = tf.transpose(output, [1, 0, 2])
        last = tf.gather(output, int(output.get_shape()[0]) - 1)
        # Softmax layer.
        weight, bias = self._weight_and_bias(
            self._num_hidden, int(self.target.get_shape()[1]))
        prediction = tf.nn.softmax(tf.matmul(last, weight) + bias)
        return prediction

    @lazy_property
    def cost(self):
        cross_entropy = tf.reduce_mean(tf.nn.softmax_cross_entropy_with_logits(self.prediction, self.target))
        #cross_entropy = -tf.reduce_sum(self.target * tf.log(self.prediction))
        return cross_entropy

    @lazy_property
    def optimize(self):
        learning_rate = 0.003
        optimizer = tf.train.RMSPropOptimizer(learning_rate)
        #optimizer = tf.train.GradientDescentOptimizer(learning_rate=learning_rate)
        #optimizer = tf.train.AdamOptimizer(learning_rate=learning_rate)
        return optimizer.minimize(self.cost)

    @lazy_property
    def accuracy(self):
        mistakes = tf.equal(
            tf.argmax(self.target, 1), tf.argmax(self.prediction, 1))
        return tf.reduce_mean(tf.cast(mistakes, tf.float32)), self.cost

    @staticmethod
    def _weight_and_bias(in_size, out_size):
        weight = tf.truncated_normal([in_size, out_size], stddev=0.01)
        bias = tf.constant(0.1, shape=[out_size])
        return tf.Variable(weight), tf.Variable(bias)

BATCH_SIZE = 128
NUM_CLASSES = 3
FEATURES = 5

if __name__ == '__main__':
    curr = sys.argv[2]
    train = CurrFeeder(True, sys.argv[1])
    test = CurrFeeder(False, sys.argv[1])

    data = tf.placeholder(tf.float32, [None, train.get_seq_max_len(), FEATURES], name='X')
    target = tf.placeholder(tf.float32, [None, NUM_CLASSES], name='Y')

    dropout = tf.placeholder(tf.float32)
    model = SequenceClassification(data, target, dropout)
    sess = tf.Session()
    sess.run(tf.initialize_all_variables())
    p = Thread(target=train.preload, args=(BATCH_SIZE, ))
    p.start()

    test.preload(BATCH_SIZE)
    test_x, test_y, _ = test.next()

    for t in tf.all_variables():
            print('Variable: %s' % t.name)

    saver = tf.train.Saver(max_to_keep=5)
    if tf.train.latest_checkpoint('rnn_saves_%s' % curr) is not None:
        print("Restoring previous RNN:", tf.train.latest_checkpoint('rnn_saves_%s' % curr))
        saver.restore(sess, tf.train.latest_checkpoint('rnn_saves_%s' % curr))
    epoch = 0
    while True:
        epoch += 1
        for _ in range(100):
            p.join()
            batch_x, batch_y, pos = train.next()
            p = Thread(target=train.preload, args=(BATCH_SIZE, ))
            p.start()
            sess.run(model.optimize, {
                data: batch_x,
                target: batch_y,
                dropout: 0.5})

        accuracy, cost = sess.run(model.accuracy, {
            data: test_x,
            target: test_y,
            dropout: 1
        })
        print('Epoch {:2d} accuracy {:3.1f}% Cost: {}'.format(epoch + 1, 100 * accuracy, cost))

        print("Saving...")
        save_path = saver.save(sess, 'rnn_saves_%s/v-rnn' % curr, global_step=epoch)
        print("Saved as:", save_path)
